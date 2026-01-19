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
- LLM provider API key for summarization (choose one):
  - `ANTHROPIC_API_KEY` for Anthropic
  - `OPENAI_API_KEY` for OpenAI
  - `OPENROUTER_API_KEY` for OpenRouter
  - `CEREBRAS_API_KEY` for Cerebras
  - `ZAI_API_KEY` for ZAI
- bird CLI requires X.com cookies in Safari/Chrome/Firefox (run `bird whoami` to verify auth)

## Data Location
`~/.xhub/xhub.db` - SQLite database with FTS5 and vector storage

## TUI Keybindings
- `j/k` nav, `g/G` top/end, `/` search, `Esc` unfocus
- `Enter` edit modal (Tab to switch fields, Enter save, Esc cancel)
- `d` delete (y confirm, n cancel)
- `o` open in browser, `1-4` toggle source filters
