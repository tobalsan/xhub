package sources

import "github.com/user/xhub/internal/db"

// Source defines the interface for bookmark sources
type Source interface {
	// Name returns the source identifier (x, raindrop, github)
	Name() string
	// Fetch retrieves bookmarks from the source
	// When incremental=true, only fetch items newer than last sync timestamp
	// When incremental=false, fetch all items (full reimport)
	Fetch(incremental bool) ([]db.Bookmark, error)
	// Available checks if the source CLI is installed
	Available() bool
}
