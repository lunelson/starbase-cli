# starbase-cli

A CLI/TUI tool for maintaining a searchable local mirror of GitHub/GitLab starred repositories, optimized for LLM-assisted development workflows.

## Overview

Stars are a curation signal that typically becomes an unmanageable inbox. **starbase** transforms them into a **personal code atlas**:

- **Shallow clones** of starred repos (configurable recency window)
- **Hybrid search** (BM25 + optional vector) over metadata, READMEs, and high-signal files
- **Bubbletea TUI** for browsing, multi-select, and actions
- **LLM-optimized exports** — paths, markdown, workspaces for feeding to coding agents
- **Multi-machine sync** — config/manifest via dotfiles; database/clones are derived state

## Installation

```bash
# Build from source
make build

# Install to $GOPATH/bin
make install

# Or add bin/ to PATH
export PATH="$PATH:$(pwd)/bin"
```

## Quick Start

```bash
# Initialize starbase directories
starbase init

# Sync starred repos from last 6 months
starbase sync --since=6mo

# Search your repos
starbase search "golang cli framework"

# Browse in TUI
starbase browse

# Export paths for LLM context
starbase export "bubbletea" --format=paths
```

## Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize directories and database |
| `sync` | Fetch stars from GitHub, clone/update repos |
| `search <query>` | BM25 search over repos |
| `list` | List all tracked repos |
| `browse` | Launch interactive TUI |
| `export <query>` | Export repo info (paths/json/yaml/markdown) |
| `collection` | Manage local collections |
| `note` | Add notes to repos |
| `pin`/`unpin` | Mark repos to prevent pruning |
| `index rebuild` | Rebuild FTS search index |

## Configuration

Config lives in `~/.config/starbase/`:

```yaml
# config.yaml
sync:
  default_window: 30d    # only sync recent stars by default
  clone_private: false   # skip private repos
  clone_archived: false  # skip archived repos

clone:
  depth: 1               # shallow clone
  single_branch: true
  skip_lfs: true

forges:
  github:
    enabled: true
```

## Authentication

starbase resolves GitHub tokens in order:
1. `STARBASE_GITHUB_TOKEN` env var
2. `gh` CLI auth (`gh auth token`)

```bash
# Recommended: use gh CLI
gh auth login

# Or set directly
export STARBASE_GITHUB_TOKEN="ghp_..."
```

## Data Layout

```
~/.config/starbase/           # Syncable (dotfiles)
├── config.yaml               # Settings
└── manifest.yaml             # Tracked repos + annotations

~/.local/share/starbase/      # Machine-local (derived)
├── starbase.db               # SQLite: cache + FTS5 index
└── clones/                   # Shallow clones
    └── github/<owner>/<repo>/
```

The **manifest** is the source of truth. Database and clones can be rebuilt on any machine via `starbase sync --full`.

## Development

```bash
# Run tests
make test

# Build
make build

# Lint (requires golangci-lint)
make lint
```

## License

MIT
