# XHub

Unified TUI for searching bookmarks across X/Twitter, Raindrop, and GitHub stars with semantic search.

## What It Does

Aggregates bookmarks from multiple sources, scrapes content, generates LLM summaries, and indexes with hybrid search (BM25 + vector embeddings).

**Sources:**
- X/Twitter bookmarks (via `bird` CLI)
- Raindrop bookmarks (via `raindrop` CLI)
- GitHub starred repos (via `gh` CLI)
- Manual URLs

## Setup

### Prerequisites

```bash
# Install CLI tools
brew install gh                    # GitHub CLI
npm install -g @raindrop/cli       # Raindrop CLI
# bird CLI: https://github.com/steipete/bird (optional)

# Set API keys
export OPENAI_API_KEY=sk-...       # Required for embeddings
export ANTHROPIC_API_KEY=sk-ant-... # Required for summaries
```

### Build

```bash
# Build with CGO (required for SQLite FTS5)
make build

# Or manually
CGO_ENABLED=1 go build -tags fts5 -o bin/xhub
```

### Configure

Create `~/.xhub/config.yaml`:

```yaml
llm:
  provider: anthropic
  model: claude-haiku-4-5-20251001

embeddings:
  provider: openai
  model: text-embedding-3-small

sources:
  x: true
  raindrop: true
  github: true
```

## Usage

### TUI (default)

```bash
xhub
```

**Keyboard shortcuts:**
- `/` - Focus search
- `j/k` or `↓/↑` - Navigate
- `g/G` - Top/bottom
- `o` - Open in browser
- `Enter` - Edit entry
- `d` - Delete (with confirm)
- `1-4` - Toggle source filters (X/Raindrop/GitHub/Manual)
- `q` - Quit

### CLI Commands

```bash
# Fetch/refresh all bookmarks
xhub fetch

# Add manual URL
xhub add https://example.com

# Search from CLI
xhub search "vector databases"
xhub search "golang tui" -j  # JSON output
xhub search "embeddings" -p  # Plaintext
```

## How It Works

1. **Fetch**: CLI tools pull bookmarks from each source
2. **Scrape**: Jina Reader API (`r.jina.ai`) fetches content
3. **Summarize**: LLM generates title, summary, keywords
4. **Embed**: OpenAI creates 1536-dim embeddings
5. **Index**: SQLite with FTS5 (BM25) + vector search
6. **Search**: Hybrid ranking via Reciprocal Rank Fusion

## Data Storage

- Database: `~/.xhub/xhub.db`
- Config: `~/.xhub/config.yaml`
- Auto-refresh: Once per 24 hours (use `xhub fetch` to force)

## Tech Stack

- **Language**: Go 1.22+
- **TUI**: Bubble Tea + Bubbles + Lip Gloss
- **Database**: SQLite with FTS5
- **Embeddings**: OpenAI text-embedding-3-small
- **LLM**: Anthropic Claude Haiku (configurable)
- **Scraper**: Jina Reader API
