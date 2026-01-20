package cmd

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/db"
	"github.com/user/xhub/internal/indexer"
)

var (
	resumVerboseFlag bool
	resumDebugFlag   bool
	resumLimitFlag  int
	resumAllFlag    bool
	resummarizeCmd = &cobra.Command{
		Use:   "resummarize",
		Short: "Regenerate summaries for existing bookmarks",
		Long:  "Re-generate LLM summaries and keywords for bookmarks that have raw content but missing summaries.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			limit := resumLimitFlag
			if resumAllFlag {
				limit = 0 // 0 means process all
			}
			return Resummarize(cfg, limit, resumVerboseFlag, resumDebugFlag)
		},
	}
)

func init() {
	resummarizeCmd.Flags().BoolVarP(&resumVerboseFlag, "verbose", "v", false, "Show detailed processing steps")
	resummarizeCmd.Flags().BoolVarP(&resumDebugFlag, "debug", "d", false, "Show raw LLM responses for debugging")
	resummarizeCmd.Flags().IntVarP(&resumLimitFlag, "limit", "l", 10, "Number of bookmarks to process (default: 10)")
	resummarizeCmd.Flags().BoolVarP(&resumAllFlag, "all", "a", false, "Process all bookmarks (overrides --limit)")
	rootCmd.AddCommand(resummarizeCmd)
}

// Resummarize regenerates summaries for bookmarks with raw content but missing summaries
func Resummarize(cfg *config.Config, limit int, verbose bool, debug bool) error {
	// Enable debug mode in summarizer
	if debug {
		indexer.SetDebugMode(true)
	}

	store, err := db.NewStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Get bookmarks with raw content but empty/missing summaries
	bookmarks, err := getBookmarksNeedingSummary(store, limit)
	if err != nil {
		return fmt.Errorf("failed to get bookmarks: %w", err)
	}

	if len(bookmarks) == 0 {
		fmt.Println("No bookmarks found needing summarization.")
		return nil
	}

	fmt.Printf("Processing %d bookmark(s)...\n\n", len(bookmarks))

	summarizer := indexer.NewSummarizer(cfg)
	embedder, err := indexer.NewEmbedder(cfg)
	if err != nil {
		fmt.Printf("Warning: embeddings disabled: %v\n", err)
		embedder = nil
	}

	successCount := 0
	for i, b := range bookmarks {
		fmt.Printf("[%d/%d] %s\n", i+1, len(bookmarks), b.URL)
		if verbose {
			fmt.Printf("  Title: %s\n", b.Title)
		}

		if b.RawContent == "" {
			fmt.Println("  Skipped: No raw content available")
			continue
		}

		// Summarize
		if verbose {
			fmt.Printf("  Summarizing...\n")
		}

		result, err := summarizer.Summarize(b.RawContent)
		if err != nil {
			fmt.Printf("  Error: summarization failed: %v\n", err)
			if verbose {
				fmt.Printf("  Raw content preview: %s\n", truncateString(b.RawContent, 200))
			}
			if debug {
				fmt.Printf("  Raw content: %s\n", truncateString(b.RawContent, 500))
			}
			continue
		}

		if result.Summary == "" {
			fmt.Println("  Error: Empty summary generated from LLM")
			if verbose {
				fmt.Printf("  Raw content preview: %s\n", truncateString(b.RawContent, 200))
			}
			if debug {
				fmt.Printf("  LLM Raw Response:\n%s\n", result.RawResponse)
			}
			continue
		}

		b.Summary = result.Summary
		b.Keywords = result.Keywords

		if verbose {
			fmt.Printf("  Summary: %s\n", result.Summary)
			fmt.Printf("  Keywords: %s\n", result.Keywords)
		}

		// Generate embedding
		if embedder != nil {
			if verbose {
				fmt.Printf("  Generating embedding...\n")
			}

			textToEmbed := b.Title + " " + b.Summary + " " + b.Keywords
			if embedding, err := embedder.Embed(textToEmbed); err != nil {
				fmt.Printf("  Warning: embedding failed: %v\n", err)
			} else {
				store.UpdateEmbedding(b.ID, embedding)
				if verbose {
					fmt.Printf("  Embedding generated (dimensions: %d)\n", len(embedding))
				}
			}
		}

		// Update bookmark
		if err := store.Update(&b); err != nil {
			fmt.Printf("  Error: failed to update bookmark: %v\n", err)
			continue
		}

		fmt.Println("  Success!")
		successCount++
		fmt.Println()
	}

	fmt.Printf("\nDone! Successfully updated %d/%d bookmark(s).\n", successCount, len(bookmarks))
	return nil
}

// getBookmarksNeedingSummary retrieves bookmarks with raw content but missing summaries
func getBookmarksNeedingSummary(store *db.Store, limit int) ([]db.Bookmark, error) {
	query := `
		SELECT id, source, url, title, summary, keywords, notes, raw_content, created_at, updated_at, scrape_status, hidden
		FROM bookmarks
		WHERE raw_content != ''
		AND (summary = '' OR summary IS NULL)
		AND hidden = 0
		ORDER BY updated_at DESC
	`

	var rows *sql.Rows
	var err error

	if limit > 0 {
		query += " LIMIT ?"
		rows, err = store.DB().Query(query, limit)
	} else {
		rows, err = store.DB().Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []db.Bookmark
	for rows.Next() {
		var b db.Bookmark
		if err := rows.Scan(
			&b.ID, &b.Source, &b.URL, &b.Title, &b.Summary, &b.Keywords, &b.Notes,
			&b.RawContent, &b.CreatedAt, &b.UpdatedAt, &b.ScrapeStatus, &b.Hidden,
		); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to scan bookmark: %v\n", err)
			continue
		}
		bookmarks = append(bookmarks, b)
	}

	return bookmarks, rows.Err()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
