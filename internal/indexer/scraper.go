package indexer

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Scraper fetches web content using Jina Reader
type Scraper struct {
	client *http.Client
}

func NewScraper() *Scraper {
	return &Scraper{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Scrape fetches the content of a URL using Jina Reader
func (s *Scraper) Scrape(targetURL string) (string, error) {
	// Jina Reader API: r.jina.ai/<url>
	jinaURL := "https://r.jina.ai/" + url.QueryEscape(targetURL)

	req, err := http.NewRequest("GET", jinaURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "text/plain")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("jina reader returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(body)

	// Limit content size to avoid excessive token usage
	const maxContentLen = 50000
	if len(content) > maxContentLen {
		content = content[:maxContentLen]
	}

	return content, nil
}
