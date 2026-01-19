package indexer

import (
	"fmt"
	"time"

	"github.com/user/xhub/internal/config"
	"github.com/user/xhub/internal/db"
	"github.com/user/xhub/internal/sources"
)

const lastRefreshKey = "last_refresh_at"

// Fetch fetches and indexes bookmarks from all enabled sources
func Fetch(cfg *config.Config, force bool) error {
	store, err := db.NewStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Check if refresh needed (once per day unless forced)
	if !force {
		lastRefresh, _ := store.GetMetadata(lastRefreshKey)
		if lastRefresh != "" {
			if t, err := time.Parse(time.RFC3339, lastRefresh); err == nil {
				if time.Since(t) < 24*time.Hour {
					fmt.Println("Already refreshed today. Use 'xhub fetch' to force refresh.")
					return nil
				}
			}
		}
	}

	// Collect enabled sources
	var srcs []sources.Source
	if cfg.Sources.GitHub {
		src := sources.NewGitHubSource()
		if src.Available() {
			srcs = append(srcs, src)
		} else {
			fmt.Println("Warning: gh CLI not found, skipping GitHub")
		}
	}
	if cfg.Sources.X {
		src := sources.NewTwitterSource()
		if src.Available() {
			srcs = append(srcs, src)
		} else {
			fmt.Println("Warning: bird CLI not found, skipping X/Twitter")
		}
	}
	if cfg.Sources.Raindrop {
		src := sources.NewRaindropSource(store)
		if src.Available() {
			srcs = append(srcs, src)
		} else {
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
		fmt.Printf("Warning: embeddings disabled: %v\n", err)
		embedder = nil
	}

	var totalItems int

	// Fetch from each source
	for _, src := range srcs {
		fmt.Printf("Fetching from %s...\n", src.Name())

		bookmarks, err := src.Fetch()
		if err != nil {
			fmt.Printf("Error fetching from %s: %v\n", src.Name(), err)
			continue
		}

		fmt.Printf("Found %d items from %s\n", len(bookmarks), src.Name())

		// Store bookmarks
		for i, b := range bookmarks {
			if err := store.Upsert(&b); err != nil {
				fmt.Printf("Error storing bookmark: %v\n", err)
				continue
			}
			printProgress(i+1, len(bookmarks), "Storing")
		}
		fmt.Println()

		totalItems += len(bookmarks)
	}

	// Process pending items (scrape, summarize, embed)
	pending, err := store.GetPending(100)
	if err != nil {
		return fmt.Errorf("failed to get pending items: %w", err)
	}

	if len(pending) > 0 {
		fmt.Printf("Processing %d pending items...\n", len(pending))

		for i, b := range pending {
			printProgress(i+1, len(pending), "Processing")

			// Scrape content
			if b.RawContent == "" {
				content, err := scraper.Scrape(b.URL)
				if err != nil {
					b.ScrapeStatus = "failed"
					store.Update(&b)
					continue
				}
				b.RawContent = content
			}

			// Summarize
			if b.Summary == "" && summarizer != nil {
				result, err := summarizer.Summarize(b.RawContent)
				if err == nil && result != nil {
					b.Summary = result.Summary
					if b.Keywords == "" {
						b.Keywords = result.Keywords
					}
				}
			}

			// Generate embedding
			if embedder != nil {
				textToEmbed := b.Title + " " + b.Summary + " " + b.Keywords
				if embedding, err := embedder.Embed(textToEmbed); err == nil {
					store.UpdateEmbedding(b.ID, embedding)
				}
			}

			b.ScrapeStatus = "success"
			b.ScrapedAt = time.Now()
			store.Update(&b)
		}
		fmt.Println()
	}

	// Update last refresh timestamp
	store.SetMetadata(lastRefreshKey, time.Now().Format(time.RFC3339))

	count, _ := store.Count()
	fmt.Printf("Done! Total items indexed: %d\n", count)

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
	if result, err := summarizer.Summarize(content); err == nil && result != nil {
		b.Summary = result.Summary
		b.Keywords = result.Keywords
	}

	// Embed
	if embedder, err := NewEmbedder(cfg); err == nil {
		textToEmbed := b.Title + " " + b.Summary + " " + b.Keywords
		if embedding, err := embedder.Embed(textToEmbed); err == nil {
			store.UpdateEmbedding(b.ID, embedding)
		}
	}

	b.ScrapeStatus = "success"
	b.ScrapedAt = time.Now()

	return store.Update(b)
}

func printProgress(current, total int, prefix string) {
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
