package sources

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

const raindropLastSyncKey = "raindrop_last_sync_ts"

type RaindropSource struct {
	store *db.Store
}

func NewRaindropSource(store *db.Store) *RaindropSource {
	return &RaindropSource{store: store}
}

func (r *RaindropSource) Name() string {
	return "raindrop"
}

func (r *RaindropSource) Available() bool {
	_, err := exec.LookPath("raindrop")
	return err == nil
}

type raindropItem struct {
	ID      int    `json:"_id"`
	Title   string `json:"title"`
	Link    string `json:"link"`
	Excerpt string `json:"excerpt"`
	Note    string `json:"note"`
	Created string `json:"created"`
	Tags    []string `json:"tags"`
}

func (r *RaindropSource) Fetch(incremental bool) ([]db.Bookmark, error) {
	// Get last sync timestamp for incremental fetch
	var lastSyncTime time.Time
	if incremental && r.store != nil {
		if ts, _ := r.store.GetMetadata(raindropLastSyncKey); ts != "" {
			lastSyncTime, _ = time.Parse(time.RFC3339, ts)
		}
	}

	var allItems []raindropItem
	var newestTime time.Time
	page := 0
	limit := 50 // max per page
	reachedOld := false

	for {
		// Raindrop CLI sorts by -created (newest first) by default
		cmd := exec.Command("raindrop", "list", "--json", "--limit", "50", "--page", itoa(page))
		output, err := cmd.Output()
		if err != nil {
			if page == 0 {
				return nil, err
			}
			break // stop on error after first page
		}

		var items []raindropItem
		if err := json.Unmarshal(output, &items); err != nil {
			var resp struct {
				Items []raindropItem `json:"items"`
			}
			if err := json.Unmarshal(output, &resp); err != nil {
				if page == 0 {
					return nil, err
				}
				break
			}
			items = resp.Items
		}

		if len(items) == 0 {
			break
		}

		for _, item := range items {
			itemTime := time.Now()
			if item.Created != "" {
				if t, err := time.Parse(time.RFC3339, item.Created); err == nil {
					itemTime = t
				}
			}
			// Truncate to seconds for consistent comparison (RFC3339 loses sub-second precision)
			itemTimeSec := itemTime.Truncate(time.Second)

			// Track newest item for next sync
			if newestTime.IsZero() || itemTimeSec.After(newestTime) {
				newestTime = itemTimeSec
			}

			// Stop if we've reached items from before last sync
			if !lastSyncTime.IsZero() && !itemTimeSec.After(lastSyncTime) {
				reachedOld = true
				break
			}

			allItems = append(allItems, item)
		}

		if reachedOld {
			break
		}

		if len(items) < limit {
			break // last page
		}
		page++
	}

	// Update last sync timestamp
	if r.store != nil && !newestTime.IsZero() {
		r.store.SetMetadata(raindropLastSyncKey, newestTime.Format(time.RFC3339))
	}

	bookmarks := make([]db.Bookmark, 0, len(allItems))
	for _, item := range allItems {
		createdAt := time.Now()
		if item.Created != "" {
			if t, err := time.Parse(time.RFC3339, item.Created); err == nil {
				createdAt = t
			}
		}

		keywords := ""
		for i, tag := range item.Tags {
			if i > 0 {
				keywords += ","
			}
			keywords += tag
		}

		bookmarks = append(bookmarks, db.Bookmark{
			Source:       "raindrop",
			URL:          item.Link,
			Title:        item.Title,
			Summary:      item.Excerpt,
			Keywords:     keywords,
			Notes:        item.Note,
			CreatedAt:    createdAt,
			ScrapeStatus: "pending",
		})
	}

	return bookmarks, nil
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
