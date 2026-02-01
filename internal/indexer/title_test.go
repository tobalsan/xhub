package indexer

import "testing"

func TestExtractTitleFromContent(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		fallback string
		want     string
	}{
		{
			name:     "prefix normalized",
			content:  "TITLE: Hello World\nBody",
			fallback: "https://example.com",
			want:     "Hello World",
		},
		{
			name:     "no prefix unchanged",
			content:  "Hello World\nBody",
			fallback: "https://example.com",
			want:     "Hello World",
		},
		{
			name:     "empty after stripping falls back",
			content:  "Title:   \nBody",
			fallback: "https://example.com",
			want:     "https://example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTitleFromContent(tc.content, tc.fallback)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
