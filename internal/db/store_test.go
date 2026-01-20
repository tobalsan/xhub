package db

import (
    "os"
    "testing"
)

func TestUpsertReturningNew(t *testing.T) {
    tmpDir, _ := os.MkdirTemp("", "xhub-test")
    defer os.RemoveAll(tmpDir)

    store, err := NewStore(tmpDir)
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.Close()

    // Test new insert
    b1 := &Bookmark{
        Source: "github",
        URL:    "https://github.com/test/repo1",
        Title:  "Test Repo 1",
    }
    isNew, err := store.UpsertReturningNew(b1)
    if err != nil {
        t.Fatalf("Failed to upsert: %v", err)
    }
    if !isNew {
        t.Error("Expected isNew=true for new insert")
    }

    // Test existing update
    b1.Title = "Updated Title"
    isNew, err = store.UpsertReturningNew(b1)
    if err != nil {
        t.Fatalf("Failed to upsert: %v", err)
    }
    if isNew {
        t.Error("Expected isNew=false for existing update")
    }
}

func TestMarkForReprocess(t *testing.T) {
    tmpDir, _ := os.MkdirTemp("", "xhub-test")
    defer os.RemoveAll(tmpDir)

    store, err := NewStore(tmpDir)
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.Close()

    // Create item with content
    b := &Bookmark{
        Source:       "github",
        URL:          "https://github.com/test/repo",
        Title:        "Test",
        ScrapeStatus: "success",
        RawContent:   "Test content",
        Summary:      "Test summary",
        Keywords:     "test, keywords",
    }
    store.Upsert(b)

    // Verify initial state
    got, _ := store.Get(b.ID)
    if got.ScrapeStatus != "success" {
        t.Fatalf("Expected success status, got %s", got.ScrapeStatus)
    }

    // Mark for reprocess
    err = store.MarkForReprocess([]string{b.ID})
    if err != nil {
        t.Fatalf("Failed to mark for reprocess: %v", err)
    }

    // Verify reprocess state
    got, _ = store.Get(b.ID)
    if got.ScrapeStatus != "pending" {
        t.Errorf("Expected pending status, got %s", got.ScrapeStatus)
    }
    if got.RawContent != "" {
        t.Error("Expected raw_content to be cleared")
    }
    if got.Summary != "" {
        t.Error("Expected summary to be cleared")
    }
    if got.Keywords != "" {
        t.Error("Expected keywords to be cleared")
    }
}
