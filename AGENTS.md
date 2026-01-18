# XHub Agent Guide

## Build & Run
```bash
make build        # Build with CGO + FTS5
./bin/xhub        # Launch TUI
./bin/xhub fetch  # Fetch from all sources
./bin/xhub search <query>     # Search (default output)
./bin/xhub search <query> -j  # JSON output
./bin/xhub search <query> -p  # Plaintext output
./bin/xhub add <url>          # Add manual URL
```

## Required: FTS5 Build Tag
Must use `-tags "fts5"` for SQLite FTS5 support. Already configured in Makefile.

## Dependencies
- `gh` CLI for GitHub stars
- `raindrop` CLI for Raindrop bookmarks
- `bird` CLI for X/Twitter bookmarks (optional)
- `OPENAI_API_KEY` for embeddings
- `ANTHROPIC_API_KEY` for summarization

## Data Location
`~/.xhub/xhub.db` - SQLite database with FTS5 and vector storage
