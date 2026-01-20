package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

const xLastSyncKey = "x_last_sync_ts"

type TwitterSource struct {
	store *db.Store
}

func NewTwitterSource(store *db.Store) *TwitterSource {
	return &TwitterSource{store: store}
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

func (t *TwitterSource) Fetch(incremental bool) ([]db.Bookmark, error) {
	// Get last sync timestamp for incremental fetch
	var lastSyncTime time.Time
	if incremental && t.store != nil {
		if ts, _ := t.store.GetMetadata(xLastSyncKey); ts != "" {
			lastSyncTime, _ = time.Parse(time.RFC3339, ts)
		}
	}

	const twitterTimeFormat = "Mon Jan 02 15:04:05 -0700 2006"
	var allTweets []birdBookmark
	var newestTime time.Time
	cursor := ""
	reachedOld := false

	// Paginate through bookmarks until we hit items older than last sync
	// Use --all --max-pages 1 to get one page at a time with nextCursor
	for !reachedOld {
		// Build command with optional cursor
		cmdStr := "bird bookmarks --all --max-pages 1 --json"
		if cursor != "" {
			cmdStr += fmt.Sprintf(" --cursor %q", cursor)
		}

		// Use temp file to avoid output truncation
		tmpFile, err := os.CreateTemp("", "bird-*.json")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		cmd := exec.Command("sh", "-c", fmt.Sprintf("%s > %s", cmdStr, tmpPath))
		if err := cmd.Run(); err != nil {
			os.Remove(tmpPath)
			return nil, fmt.Errorf("bird bookmarks failed: %w", err)
		}

		output, err := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read bird output: %w", err)
		}

		var resp birdResponse
		if err := json.Unmarshal(output, &resp); err != nil {
			// Try parsing as direct array (fallback for older versions)
			var tweets []birdBookmark
			if arrErr := json.Unmarshal(output, &tweets); arrErr != nil {
				return nil, fmt.Errorf("failed to parse bird output: %w", err)
			}
			resp.Tweets = tweets
		}

		if len(resp.Tweets) == 0 {
			break
		}

		for _, tweet := range resp.Tweets {
			var tweetTime time.Time
			if tweet.CreatedAt != "" {
				if parsed, err := time.Parse(twitterTimeFormat, tweet.CreatedAt); err == nil {
					tweetTime = parsed.Truncate(time.Second)
				}
			}

			// Track newest time for metadata update
			if newestTime.IsZero() || tweetTime.After(newestTime) {
				newestTime = tweetTime
			}

			// If incremental and this tweet is at or before last sync, stop
			if !lastSyncTime.IsZero() && !tweetTime.After(lastSyncTime) {
				reachedOld = true
				break
			}

			allTweets = append(allTweets, tweet)
		}

		// If no more pages or we've reached old items, stop
		if resp.NextCursor == "" || reachedOld {
			break
		}
		cursor = resp.NextCursor
	}

	// Update last sync timestamp
	if t.store != nil && !newestTime.IsZero() {
		t.store.SetMetadata(xLastSyncKey, newestTime.Format(time.RFC3339))
	}

	// Convert to bookmarks
	bookmarks := make([]db.Bookmark, 0, len(allTweets))
	for _, tweet := range allTweets {
		createdAt := time.Now()
		if tweet.CreatedAt != "" {
			if parsed, err := time.Parse(twitterTimeFormat, tweet.CreatedAt); err == nil {
				createdAt = parsed
			}
		}

		title := tweet.Text
		if len(title) > 100 {
			title = title[:100] + "..."
		}

		url := fmt.Sprintf("https://x.com/%s/status/%s", tweet.Author.Username, tweet.ID)

		bookmarks = append(bookmarks, db.Bookmark{
			Source:       "x",
			URL:          url,
			Title:        title,
			RawContent:   tweet.Text,
			CreatedAt:    createdAt,
			ScrapeStatus: "success",
		})
	}

	return bookmarks, nil
}
