package sources

import (
	"encoding/json"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

type RaindropSource struct{}

func NewRaindropSource() *RaindropSource {
	return &RaindropSource{}
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

func (r *RaindropSource) Fetch() ([]db.Bookmark, error) {
	// Use raindrop CLI to list bookmarks
	cmd := exec.Command("raindrop", "list", "--json")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var items []raindropItem
	if err := json.Unmarshal(output, &items); err != nil {
		// Try parsing as object with items array
		var resp struct {
			Items []raindropItem `json:"items"`
		}
		if err := json.Unmarshal(output, &resp); err != nil {
			return nil, err
		}
		items = resp.Items
	}

	bookmarks := make([]db.Bookmark, 0, len(items))
	for _, item := range items {
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
