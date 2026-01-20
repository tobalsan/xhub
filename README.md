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

# LLM provider for summarization (choose one):
export ANTHROPIC_API_KEY=sk-ant-...    # Anthropic
export OPENROUTER_API_KEY=sk-or-...    # OpenRouter
export CEREBRAS_API_KEY=...            # Cerebras
export ZAI_API_KEY=...                 # Z.AI
export GEMINI_API_KEY=...              # Google Gemini
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

**API Keys**: You can either set environment variables or add `api_key` directly in the config file. Environment variables take precedence over config values.

#### LLM Provider Options

**Anthropic (default)**
```yaml
llm:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  api_key: sk-ant-...
```
Set `ANTHROPIC_API_KEY` environment variable or `api_key` in config.

**OpenRouter**
```yaml
llm:
  provider: openrouter
  model: anthropic/claude-haiku-4-5-20251001
  base_url: https://openrouter.ai/api/v1
  api_key: sk-or-...
```
Set `OPENROUTER_API_KEY` environment variable or `api_key` in config.

**Cerebras**
```yaml
llm:
  provider: cerebras
  model: llama3.1-70b
  base_url: https://api.cerebras.ai/v1
  api_key: csk-...
```
Set `CEREBRAS_API_KEY` environment variable or `api_key` in config.

**Z.AI**
```yaml
llm:
  provider: zai
  model: <model-name>
  base_url: https://<zai-endpoint>/v1
  api_key: <your-api-key>
```
Set `ZAI_API_KEY` environment variable or `api_key` in config.

**Google Gemini**
```yaml
llm:
  provider: gemini
  model: gemini-1.5-flash
  base_url: https://generativelanguage.googleapis.com/v1beta/openai/
  api_key: <your-api-key>
```
Set `GEMINI_API_KEY` environment variable or `api_key` in config.

**Embeddings (OpenAI)**
```yaml
embeddings:
  provider: openai
  model: text-embedding-3-small
  api_key: sk-...
```
Set `OPENAI_API_KEY` environment variable or `api_key` in config.

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
