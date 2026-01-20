package indexer

import (
    "os"
    "testing"

    "github.com/user/xhub/internal/db"
)

func TestFetchOptionsReprocess(t *testing.T) {
    tmpDir, _ := os.MkdirTemp("", "xhub-test")
    defer os.RemoveAll(tmpDir)

    store, err := db.NewStore(tmpDir)
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.Close()

    // Create existing item with content
    b := &db.Bookmark{
        Source:       "github",
        URL:          "https://github.com/test/repo",
        Title:        "Test",
        ScrapeStatus: "success",
        RawContent:   "Existing content",
        Summary:      "Existing summary",
        Keywords:     "existing",
    }
    store.Upsert(b)

    // Simulate what happens during fetch with reprocess
    // 1. Upsert returns isNew=false for existing
    isNew, _ := store.UpsertReturningNew(b)
    if isNew {
        t.Error("Expected existing item to return isNew=false")
    }

    // 2. When reprocess=true, we collect IDs and mark for reprocess
    opts := FetchOptions{Force: true, Reprocess: true}
    if opts.Reprocess {
        store.MarkForReprocess([]string{b.ID})
    }

    // 3. Verify item is now pending with cleared content
    got, _ := store.Get(b.ID)
    if got.ScrapeStatus != "pending" {
        t.Errorf("Expected pending, got %s", got.ScrapeStatus)
    }
    if got.RawContent != "" || got.Summary != "" {
        t.Error("Expected content to be cleared")
    }

    // 4. GetPending should now return this item
    pending, _ := store.GetPending(100)
    found := false
    for _, p := range pending {
        if p.ID == b.ID {
            found = true
            break
        }
    }
    if !found {
        t.Error("Expected item to appear in GetPending")
    }
}
