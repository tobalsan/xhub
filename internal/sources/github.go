package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

const githubLastSyncKey = "github_last_sync_ts"

type GitHubSource struct {
	store *db.Store
}

func NewGitHubSource(store *db.Store) *GitHubSource {
	return &GitHubSource{store: store}
}

func (g *GitHubSource) Name() string {
	return "github"
}

func (g *GitHubSource) Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

type ghStar struct {
	StarredAt string `json:"starred_at"`
	Repo      struct {
		FullName    string `json:"full_name"`
		HTMLURL     string `json:"html_url"`
		Description string `json:"description"`
	} `json:"repo"`
}

func (g *GitHubSource) Fetch() ([]db.Bookmark, error) {
	// Get last sync timestamp for incremental fetch
	var lastSyncTime time.Time
	if g.store != nil {
		if ts, _ := g.store.GetMetadata(githubLastSyncKey); ts != "" {
			lastSyncTime, _ = time.Parse(time.RFC3339, ts)
		}
	}

	var allStars []ghStar
	var newestTime time.Time
	page := 1
	perPage := 100 // max per page
	reachedOld := false

	for {
		// Paginate manually to support early exit on incremental fetch
		// sort=created&direction=desc gives newest first (default)
		cmd := exec.Command("gh", "api",
			fmt.Sprintf("user/starred?sort=created&direction=desc&per_page=%d&page=%d", perPage, page),
			"-H", "Accept: application/vnd.github.star+json")

		output, err := cmd.Output()
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break // stop on error after first page
		}

		var stars []ghStar
		if err := json.Unmarshal(output, &stars); err != nil {
			// Try concatenated JSON arrays (fallback for older gh versions)
			stars, err = parseMultipleArrays(output)
			if err != nil {
				if page == 1 {
					return nil, err
				}
				break
			}
		}

		if len(stars) == 0 {
			break
		}

		for _, star := range stars {
			starTime := time.Now()
			if star.StarredAt != "" {
				if t, err := time.Parse(time.RFC3339, star.StarredAt); err == nil {
					starTime = t
				}
			}
			// Truncate to seconds for consistent comparison (RFC3339 loses sub-second precision)
			starTimeSec := starTime.Truncate(time.Second)

			// Track newest item for next sync
			if newestTime.IsZero() || starTimeSec.After(newestTime) {
				newestTime = starTimeSec
			}

			// Stop if we've reached items from before last sync
			if !lastSyncTime.IsZero() && !starTimeSec.After(lastSyncTime) {
				reachedOld = true
				break
			}

			allStars = append(allStars, star)
		}

		if reachedOld {
			break
		}

		if len(stars) < perPage {
			break // last page
		}
		page++
	}

	// Update last sync timestamp
	if g.store != nil && !newestTime.IsZero() {
		g.store.SetMetadata(githubLastSyncKey, newestTime.Format(time.RFC3339))
	}

	bookmarks := make([]db.Bookmark, 0, len(allStars))
	for _, star := range allStars {
		createdAt := time.Now()
		if star.StarredAt != "" {
			if t, err := time.Parse(time.RFC3339, star.StarredAt); err == nil {
				createdAt = t
			}
		}

		bookmarks = append(bookmarks, db.Bookmark{
			Source:       "github",
			URL:          star.Repo.HTMLURL,
			Title:        star.Repo.FullName,
			Summary:      star.Repo.Description,
			CreatedAt:    createdAt,
			ScrapeStatus: "pending",
		})
	}

	return bookmarks, nil
}

// parseMultipleArrays handles gh paginate output which can be concatenated arrays
func parseMultipleArrays(data []byte) ([]ghStar, error) {
	var result []ghStar
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var page []ghStar
		if err := decoder.Decode(&page); err != nil {
			return nil, err
		}
		result = append(result, page...)
	}
	return result, nil
}
