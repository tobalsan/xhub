package sources

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/user/xhub/internal/db"
)

type GitHubSource struct{}

func NewGitHubSource() *GitHubSource {
	return &GitHubSource{}
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
	// Use gh api with --paginate and --slurp to merge all pages into single array
	cmd := exec.Command("gh", "api", "--paginate", "--slurp", "user/starred",
		"-H", "Accept: application/vnd.github.star+json")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// --slurp wraps paginated results in an outer array: [[page1], [page2], ...]
	var pages [][]ghStar
	if err := json.Unmarshal(output, &pages); err != nil {
		// Try without outer array (single page or no --slurp)
		var stars []ghStar
		if err := json.Unmarshal(output, &stars); err != nil {
			// Try concatenated JSON arrays (fallback for older gh versions)
			stars, err = parseMultipleArrays(output)
			if err != nil {
				return nil, err
			}
		}
		pages = [][]ghStar{stars}
	}

	// Flatten pages
	var allStars []ghStar
	for _, page := range pages {
		allStars = append(allStars, page...)
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
