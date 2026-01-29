# starbase-cli

A CLI/TUI tool for maintaining a searchable local mirror of GitHub/GitLab starred repositories, optimized for LLM-assisted development workflows.

## Overview

Stars are a curation signal that typically becomes an unmanageable inbox. **starbase** transforms them into a **personal code atlas**:

- **Shallow clones** of starred repos (configurable recency window)
- **Hybrid search** (BM25 + optional vector) over metadata, READMEs, and high-signal files
- **Bubbletea TUI** for browsing, search, and actions
- **LLM-optimized exports** — paths, markdown, workspaces for feeding to coding agents
- **Multi-machine sync** — config/manifest via dotfiles; database/clones are derived state

## Installation

```bash
go install github.com/lunelson/starbase-cli/cmd/starbase@latest
```

Or build from source:

```bash
git clone https://github.com/lunelson/starbase-cli
cd starbase-cli
make build
```

## Quick Start

```bash
# Initialize starbase
starbase init

# Sync your starred repos (last 30 days by default)
starbase sync

# Sync all stars
starbase sync --full

# Search your repos
starbase search "tui framework"

# Browse with TUI
starbase browse

# Export paths for feeding to LLM
starbase export --format=paths | xargs -I {} cat {}/README.md
```

## Commands

### Core Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize starbase directories and database |
| `sync` | Sync starred repos from forges |
| `search <query>` | Search tracked repositories |
| `list` | List all tracked repositories |
| `browse` | Launch interactive TUI |

### Export & Annotation

| Command | Description |
|---------|-------------|
| `export [query]` | Export repo information |
| `collection` | Manage local collections |
| `note <query> [text]` | Add/view notes on repos |
| `pin <query>` | Pin repos to prevent pruning |
| `unpin <query>` | Unpin repos |

### Index Management

| Command | Description |
|---------|-------------|
| `index rebuild` | Rebuild search index |
| `index stats` | Show index statistics |

## Configuration

Configuration lives in `~/.config/starbase/config.yaml`:

```yaml
version: 1

clone:
  depth: 1
  single_branch: true
  skip_submodules: true
  skip_lfs: true

sync:
  default_window: 30d
  clone_missing: true
  clone_private: false
  clone_archived: false
  max_repos_per_sync: 100

search:
  engine: bm25
  default_limit: 20

forges:
  github:
    enabled: true
  gitlab:
    enabled: false
```

## Authentication

starbase uses the following token resolution order:

1. `STARBASE_GITHUB_TOKEN` environment variable
2. `GITHUB_TOKEN` environment variable  
3. `gh` CLI (if authenticated)

## Data Layout

```
~/.config/starbase/           # Syncable (dotfiles)
├── config.yaml
└── manifest.yaml

~/.local/share/starbase/      # Machine-local
├── starbase.db
├── cache/
└── clones/
    └── github/<owner>/<repo>/
```

## Search Query Syntax

| Filter | Example | Description |
|--------|---------|-------------|
| Free text | `bubbletea tui` | Search all indexed fields |
| `lang:` | `lang:go` | Filter by primary language |
| `topic:` | `topic:cli` | Filter by topic/tag |

## Output Formats

Export supports multiple formats:

```bash
starbase export --format=paths      # Local paths only
starbase export --format=json       # Full JSON
starbase export --format=yaml       # Full YAML  
starbase export --format=markdown   # Markdown documentation
```

## TUI Key Bindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `/` | Focus search |
| `Enter` | Select / Execute search |
| `Esc` | Cancel search |
| `q` | Quit |

## License

MIT
