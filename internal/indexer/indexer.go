package indexer

import (
	"fmt"
	"time"

	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/db"
	"github.com/user/xhub/internal/sources"
)

const lastRefreshKey = "last_refresh_at"

// FetchOptions configures fetch behavior
type FetchOptions struct {
	Force     bool     // Full reimport (vs incremental)
	Reprocess bool     // Re-scrape, re-summarize, re-embed existing items
	Verbose   bool     // Show detailed processing steps
	Silent    bool     // Suppress all output (for TUI background refresh)
	Sources   []string // Filter to specific sources (empty = all)
}

// Fetch fetches and indexes bookmarks from enabled sources
func Fetch(cfg *config.Config, opts FetchOptions) error {
	store, err := db.NewStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Build source filter set
	sourceFilter := make(map[string]bool)
	for _, s := range opts.Sources {
		sourceFilter[s] = true
	}
	filterEnabled := len(sourceFilter) > 0

	// Helper to check if source is enabled
	sourceEnabled := func(name string) bool {
		if !filterEnabled {
			return true
		}
		return sourceFilter[name]
	}

	// Collect enabled sources
	var srcs []sources.Source
	if cfg.Sources.GitHub && sourceEnabled("github") {
		src := sources.NewGitHubSource(store)
		if src.Available() {
			srcs = append(srcs, src)
		} else if !opts.Silent {
			fmt.Println("Warning: gh CLI not found, skipping GitHub")
		}
	}
	if cfg.Sources.X && sourceEnabled("x") {
		src := sources.NewTwitterSource(store)
		if src.Available() {
			srcs = append(srcs, src)
		} else if !opts.Silent {
			fmt.Println("Warning: bird CLI not found, skipping X/Twitter")
		}
	}
	if cfg.Sources.Raindrop && sourceEnabled("raindrop") {
		src := sources.NewRaindropSource(store)
		if src.Available() {
			srcs = append(srcs, src)
		} else if !opts.Silent {
			fmt.Println("Warning: raindrop CLI not found, skipping Raindrop")
		}
	}

	if len(srcs) == 0 {
		return fmt.Errorf("no sources available")
	}

	// Initialize components
	scraper := NewScraper()
	summarizer := NewSummarizer(cfg)
	embedder, err := NewEmbedder(cfg)
	if err != nil {
		if !opts.Silent {
			fmt.Printf("Warning: embeddings disabled: %v\n", err)
		}
		embedder = nil
	}

	var totalItems int
	var totalNewItems int

	// Fetch from each source
	// incremental = !force (default is incremental)
	incremental := !opts.Force

	// Per-source stats
	type sourceStats struct {
		newItems     int
		skippedItems int
	}
	stats := make(map[string]*sourceStats)

	for _, src := range srcs {
		if !opts.Silent {
			fmt.Printf("Fetching from %s...\n", src.Name())
		}

		bookmarks, err := src.Fetch(incremental)
		if err != nil {
			if !opts.Silent {
				fmt.Printf("Error fetching from %s: %v\n", src.Name(), err)
			}
			continue
		}

		stats[src.Name()] = &sourceStats{}

		// Store bookmarks and track new vs existing
		var idsToReprocess []string
		for i, b := range bookmarks {
			isNew, err := store.UpsertReturningNew(&b)
			if err != nil {
				if !opts.Silent {
					fmt.Printf("Error storing bookmark: %v\n", err)
				}
				continue
			}
			if isNew {
				stats[src.Name()].newItems++
			} else {
				stats[src.Name()].skippedItems++
				// If reprocessing, collect existing item IDs
				if opts.Reprocess {
					idsToReprocess = append(idsToReprocess, b.ID)
				}
			}
			printProgress(i+1, len(bookmarks), "Storing", opts.Silent)
		}
		if !opts.Silent {
			fmt.Println()
		}

		// Mark existing items for reprocessing if requested
		if opts.Reprocess && len(idsToReprocess) > 0 {
			if err := store.MarkForReprocess(idsToReprocess); err != nil {
				if !opts.Silent {
					fmt.Printf("Warning: could not mark items for reprocessing: %v\n", err)
				}
			}
		}

		// On force fetch, detect and delete orphaned items
		if opts.Force {
			urls := make([]string, len(bookmarks))
			for i, b := range bookmarks {
				urls[i] = b.URL
			}

			orphans, err := store.GetOrphanedBySource(src.Name(), urls)
			if err != nil {
				if !opts.Silent {
					fmt.Printf("Warning: could not check for orphans: %v\n", err)
				}
			} else if len(orphans) > 0 {
				if !opts.Silent {
					fmt.Printf("Removing %d orphaned items from %s:\n", len(orphans), src.Name())
				}
				for _, o := range orphans {
					if !opts.Silent {
						fmt.Printf("  - %s\n", o.URL)
					}
					if err := store.Delete(o.ID); err != nil {
						if !opts.Silent {
							fmt.Printf("    Error deleting: %v\n", err)
						}
					}
				}
			}
		}

		totalItems += len(bookmarks)
		totalNewItems += stats[src.Name()].newItems
	}

	// Print per-source delta stats
	if !opts.Silent {
		fmt.Println()
		for name, s := range stats {
			fmt.Printf("Found %d new %s items, skipped %d existing\n", s.newItems, name, s.skippedItems)
		}
	}

	// Process pending items (scrape, summarize, embed)
	// Only process if we have new items or --reprocess was requested
	shouldProcess := totalNewItems > 0 || opts.Reprocess
	if shouldProcess {
		pending, err := store.GetPending(100)
		if err != nil {
			return fmt.Errorf("failed to get pending items: %w", err)
		}

		if len(pending) > 0 {
			if !opts.Silent {
				fmt.Printf("Processing %d pending items...\n", len(pending))
			}

			for i, b := range pending {
				printProgress(i+1, len(pending), "Processing", opts.Silent)

				// Scrape content
				if b.RawContent == "" {
					if opts.Verbose && !opts.Silent {
						fmt.Printf("\n  Scraping: %s\n", b.URL)
					}
					content, err := scraper.Scrape(b.URL)
					if err != nil {
						if opts.Verbose && !opts.Silent {
							fmt.Printf("  Scraping failed: %v\n", err)
						}
						b.ScrapeStatus = "failed"
						store.Update(&b)
						continue
					}
					b.RawContent = content
					if opts.Verbose && !opts.Silent {
						fmt.Printf("  Scraped %d characters\n", len(content))
					}
				}

				// Summarize
				if b.Summary == "" && summarizer != nil {
					if opts.Verbose && !opts.Silent {
						fmt.Printf("  Summarizing...\n")
					}
					result, err := summarizer.Summarize(b.RawContent)
					if err != nil {
						if !opts.Silent {
							fmt.Printf("Warning: summarization failed for %s: %v\n", b.URL, err)
						}
					} else if result != nil {
						b.Summary = result.Summary
						if b.Keywords == "" {
							b.Keywords = result.Keywords
						}
						if opts.Verbose && !opts.Silent {
							fmt.Printf("  Summary: %s\n", result.Summary)
							fmt.Printf("  Keywords: %s\n", result.Keywords)
						}
					}
				}

				// Generate embedding
				if embedder != nil {
					if opts.Verbose && !opts.Silent {
						fmt.Printf("  Generating embedding...\n")
					}
					textToEmbed := b.Title + " " + b.Summary + " " + b.Keywords
					if embedding, err := embedder.Embed(textToEmbed); err != nil {
						if !opts.Silent {
							fmt.Printf("Warning: embedding failed for %s: %v\n", b.URL, err)
						}
					} else {
						store.UpdateEmbedding(b.ID, embedding)
						if opts.Verbose && !opts.Silent {
							fmt.Printf("  Embedding generated (dimensions: %d)\n", len(embedding))
						}
					}
				}

				b.ScrapeStatus = "success"
				b.ScrapedAt = time.Now()
				store.Update(&b)
			}
			if !opts.Silent {
				fmt.Println()
			}
		}
	}

	// Update last refresh timestamp
	store.SetMetadata(lastRefreshKey, time.Now().Format(time.RFC3339))

	if !opts.Silent {
		count, _ := store.Count()
		fmt.Printf("Done! Total items indexed: %d\n", count)
	}

	return nil
}

// AddManualURL adds a manual URL to the index
func AddManualURL(cfg *config.Config, url string) error {
	store, err := db.NewStore(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	// Check if already exists
	if existing, _ := store.GetByURL(url); existing != nil {
		return fmt.Errorf("URL already indexed")
	}

	b := &db.Bookmark{
		Source:       "manual",
		URL:          url,
		Title:        url, // Will be updated after scraping
		ScrapeStatus: "pending",
	}

	if err := store.Upsert(b); err != nil {
		return err
	}

	// Try to scrape and process immediately
	scraper := NewScraper()
	content, err := scraper.Scrape(url)
	if err != nil {
		fmt.Printf("Warning: could not scrape URL: %v\n", err)
		return nil
	}

	b.RawContent = content

	// Extract title from content (first line usually)
	if len(content) > 0 {
		lines := []rune(content)
		end := 100
		if len(lines) < end {
			end = len(lines)
		}
		for i, r := range lines[:end] {
			if r == '\n' {
				end = i
				break
			}
		}
		b.Title = string(lines[:end])
	}

	// Summarize
	summarizer := NewSummarizer(cfg)
	result, err := summarizer.Summarize(content)
	if err != nil {
		fmt.Printf("Warning: summarization failed: %v\n", err)
	} else if result != nil {
		b.Summary = result.Summary
		b.Keywords = result.Keywords
	}

	// Embed
	embedder, errEmbed := NewEmbedder(cfg)
	if errEmbed != nil {
		fmt.Printf("Warning: embedder not available: %v\n", errEmbed)
	} else {
		textToEmbed := b.Title + " " + b.Summary + " " + b.Keywords
		if embedding, err := embedder.Embed(textToEmbed); err != nil {
			fmt.Printf("Warning: embedding failed: %v\n", err)
		} else {
			store.UpdateEmbedding(b.ID, embedding)
		}
	}

	b.ScrapeStatus = "success"
	b.ScrapedAt = time.Now()

	return store.Update(b)
}

func printProgress(current, total int, prefix string, silent bool) {
	if silent {
		return
	}
	pct := float64(current) / float64(total) * 100
	barWidth := 30
	filled := int(float64(barWidth) * float64(current) / float64(total))

	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	fmt.Printf("\r%s [%s] %d/%d (%.0f%%)", prefix, bar, current, total, pct)
}
