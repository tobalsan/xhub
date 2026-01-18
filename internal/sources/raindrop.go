package sources

import (
	"encoding/json"
	"fmt"
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
	var allItems []raindropItem
	page := 0
	limit := 50 // max per page

	for {
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

		allItems = append(allItems, items...)

		if len(items) < limit {
			break // last page
		}
		page++
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
