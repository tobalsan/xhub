package db

import "time"

type Bookmark struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"` // x, raindrop, github, manual
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary,omitempty"`
	Keywords     string    `json:"keywords,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	RawContent   string    `json:"raw_content,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ScrapedAt    time.Time `json:"scraped_at,omitempty"`
	ScrapeStatus string    `json:"scrape_status"` // success, pending, failed
	Hidden       bool      `json:"hidden"`
}

type SearchResult struct {
	Bookmark
	Score float64 `json:"score"`
}
