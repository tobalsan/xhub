package sources

import (
	"encoding/json"
	"fmt"
	"os"
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

// birdBookmark matches the JSON schema from bird CLI --json output
type birdBookmark struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
	Author    struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
}

// birdResponse handles paginated response: { tweets: [...], nextCursor: "..." }
type birdResponse struct {
	Tweets     []birdBookmark `json:"tweets"`
	NextCursor string         `json:"nextCursor"`
}

func (t *TwitterSource) Fetch() ([]db.Bookmark, error) {
	// Use bird CLI to fetch all bookmarks with --all --json
	// bird bookmarks --all --json returns { tweets: [...], nextCursor: "..." }
	//
	// NOTE: We write to a temp file because bird CLI output can exceed 64KB,
	// and Go's exec.Command().Output() truncates large outputs from some CLIs.
	tmpFile, err := os.CreateTemp("", "bird-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("bird bookmarks --all --json > %s", tmpPath))
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bird bookmarks failed: %w", err)
	}

	output, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bird output: %w", err)
	}

	// Parse response - bird returns { tweets: [...], nextCursor: "..." } when using --all
	var resp birdResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		// Try parsing as direct array (fallback for older versions)
		var tweets []birdBookmark
		if arrErr := json.Unmarshal(output, &tweets); arrErr != nil {
			return nil, fmt.Errorf("failed to parse bird output: %w", err)
		}
		resp.Tweets = tweets
	}

	bookmarks := make([]db.Bookmark, 0, len(resp.Tweets))
	for _, tweet := range resp.Tweets {
		createdAt := time.Now()
		if tweet.CreatedAt != "" {
			// bird uses Twitter's Ruby-style format: "Mon Jan 02 15:04:05 +0000 2006"
			const twitterTimeFormat = "Mon Jan 02 15:04:05 -0700 2006"
			if parsed, err := time.Parse(twitterTimeFormat, tweet.CreatedAt); err == nil {
				createdAt = parsed
			}
		}

		title := tweet.Text
		if len(title) > 100 {
			title = title[:100] + "..."
		}

		// Construct tweet URL from author username and tweet ID
		url := fmt.Sprintf("https://x.com/%s/status/%s", tweet.Author.Username, tweet.ID)

		bookmarks = append(bookmarks, db.Bookmark{
			Source:       "x",
			URL:          url,
			Title:        title,
			RawContent:   tweet.Text,
			CreatedAt:    createdAt,
			ScrapeStatus: "success", // Tweet content is already available
		})
	}

	return bookmarks, nil
}
