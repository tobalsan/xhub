package sources

import (
	"encoding/json"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

type TwitterSource struct{}

func NewTwitterSource() *TwitterSource {
	return &TwitterSource{}
}

func (t *TwitterSource) Name() string {
	return "x"
}

func (t *TwitterSource) Available() bool {
	_, err := exec.LookPath("bird")
	return err == nil
}

type birdBookmark struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
}

func (t *TwitterSource) Fetch() ([]db.Bookmark, error) {
	// Use bird CLI to get bookmarks
	// Command TBD based on bird CLI interface - trying common patterns
	cmd := exec.Command("bird", "bookmarks", "--json")

	output, err := cmd.Output()
	if err != nil {
		// Try alternative command
		cmd = exec.Command("bird", "bookmarks", "list", "--json")
		output, err = cmd.Output()
		if err != nil {
			return nil, err
		}
	}

	var tweets []birdBookmark
	if err := json.Unmarshal(output, &tweets); err != nil {
		return nil, err
	}

	bookmarks := make([]db.Bookmark, 0, len(tweets))
	for _, tweet := range tweets {
		createdAt := time.Now()
		if tweet.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, tweet.CreatedAt); err == nil {
				createdAt = t
			}
		}

		title := tweet.Text
		if len(title) > 100 {
			title = title[:100] + "..."
		}

		bookmarks = append(bookmarks, db.Bookmark{
			Source:       "x",
			URL:          tweet.URL,
			Title:        title,
			RawContent:   tweet.Text,
			CreatedAt:    createdAt,
			ScrapeStatus: "success", // Tweet content is already available
		})
	}

	return bookmarks, nil
}
