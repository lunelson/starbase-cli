# starbase-cli Specification

> A CLI/TUI tool for maintaining a searchable local mirror of GitHub/GitLab starred repositories, optimized for LLM-assisted development workflows.

## Overview

Stars are a curation signal that typically becomes an unmanageable inbox. **starbase-cli** transforms them into a **personal code atlas**:

- **Shallow clones** of starred repos (configurable recency window)
- **Hybrid search** (BM25 + optional vector) over metadata, READMEs, and high-signal files
- **Bubbletea TUI** for browsing, multi-select, and actions
- **LLM-optimized exports** — paths, markdown, workspaces for feeding to coding agents
- **Multi-machine sync** — config/manifest via dotfiles; database/clones are derived state

## Core Concepts

### Starbase

The local environment containing all managed state:

- **Config** (`~/.config/starbase/config.yaml`) — Settings, policies, forge auth references
- **Manifest** (`~/.config/starbase/manifest.yaml`) — Source of truth for tracked repos + annotations
- **Database** (`~/.local/share/starbase/starbase.db`) — Cache, FTS index, embeddings
- **Clones** (`~/.local/share/starbase/clones/`) — Shallow git clones

### Manifest as Intent, Database as Cache

The manifest declares *what* should be tracked and *how* it's annotated. The database and clones are derived state that can be rebuilt on any machine via `starbase sync --full`.

This enables multi-machine sync: version the config/manifest in dotfiles; regenerate local state per machine.

### Star Record

Internal representation of a tracked repository:

```yaml
forge: github | gitlab
forge_id: string          # Stable ID (GitHub node ID, GitLab project ID)
owner: string
name: string
url: string               # Clone URL
web_url: string           # Browser URL
local_path: string | null # Path to clone, null if not yet cloned
starred_at: datetime | null
synced_at: datetime | null
status: active | archived | unavailable | pending
```

### Collections

Local-only groupings independent of forge stars:

- Don't affect GitHub/GitLab state
- Enable workflows: "prompt packs", "study sets", "client projects"
- Persist in manifest, cache in database

## Commands

### Initialization & Auth

```
starbase init                     Initialize starbase directories and config
starbase auth login [--forge=X]   Authenticate with forge (device flow or token)
starbase auth status              Show authentication status
starbase auth logout [--forge=X]  Clear stored credentials
```

### Sync & Clone Management

```
starbase sync                     Sync stars and update clones
  --full                          Clone all stars, not just recent window
  --metadata-only                 Skip git operations, update API data only
  --pull-only                     Update existing clones only
  --since=DURATION                Override recency window (e.g., 30d, 1w)
  --dry-run                       Show plan without executing

starbase add <url>                Clone repo, star on forge, add to manifest
  --no-star                       Don't star on forge

starbase remove <query>           Remove local clone
  --unstar                        Also unstar on forge
  --yes                           Skip confirmation
```

### Search & Discovery

```
starbase search <query>           Search tracked repos
  --limit=N                       Max results (default: 20)
  --json | --markdown | --yaml    Output format
  
starbase list                     List all tracked repos
  --format=FORMAT                 Output format
  --collection=NAME               Filter by collection
  --status=STATUS                 Filter by status
```

#### Query Syntax

| Filter | Example | Description |
|--------|---------|-------------|
| Free text | `bubbletea tui` | Search all indexed fields |
| `lang:` | `lang:go` | Filter by primary language |
| `topic:` | `topic:cli` | Filter by topic/tag |
| `has:` | `has:dockerfile` | Filter by presence of file |
| `collection:` | `collection:llm-refs` | Filter by collection |
| `status:` | `status:archived` | Filter by status |

### Export

```
starbase export <query>           Export repo information
  --format=FORMAT                 paths | json | markdown | yaml | workspace
  --include-readme                Include README content
  --output=PATH                   Write to file instead of stdout
```

#### Workspace Export

`--format=workspace` copies selected repos to a target directory and generates a `CONTEXT.md` manifest—ideal for LLM context injection.

### Collections & Annotations

```
starbase collection list
starbase collection create <name>
starbase collection delete <name>
starbase collection add <collection> <query>
starbase collection remove <collection> <query>

starbase note <query> <text>      Add/update note on repo
starbase pin <query>              Prevent repo from being pruned
starbase unpin <query>
```

### Maintenance

```
starbase prune                    Remove clones per policy
  --max-disk=SIZE                 Remove until under size limit
  --older-than=DURATION           Remove repos not synced in N days
  --keep-pinned                   Never prune pinned repos
  --dry-run                       Show what would be pruned

starbase index rebuild            Rebuild FTS index
starbase index rebuild-embeddings Rebuild vector embeddings
```

### TUI

```
starbase browse                   Launch interactive TUI
```

### MCP Server (Future)

```
starbase serve                    Start MCP server
  --port=PORT                     Server port
```

## Configuration

### config.yaml

```yaml
version: 1

# Machine-local data directory
data_dir: ~/.local/share/starbase

# Clone behavior
clone:
  depth: 1
  single_branch: true
  skip_submodules: true
  skip_lfs: true

# Sync policies
sync:
  default_window: 30d
  clone_missing: true
  clone_private: false
  clone_archived: false
  max_repos_per_sync: 100
  reset_on_conflict: true

# Index settings
index:
  readme: true
  high_signal_files:
    - go.mod
    - package.json
    - Cargo.toml
    - pyproject.toml
    - Dockerfile
    - docker-compose.yml
  max_file_size_kb: 100

# Search settings
search:
  engine: bm25              # bm25 | hybrid
  embedding_model: nomic-embed-text
  embedding_provider: ollama
  default_limit: 20

# Ranking signal weights
ranking:
  star_recency_weight: 0.2
  push_recency_weight: 0.1
  language_match_weight: 0.15

# Forge configuration
forges:
  github:
    enabled: true
  gitlab:
    enabled: false
    host: gitlab.com

# Export defaults
export:
  default_format: paths
  include_readme: false

# Pruning policies
prune:
  max_total_size_gb: 50
  max_repo_age_days: 365
  keep_pinned: true
```

### manifest.yaml

```yaml
version: 1

collections:
  - name: llm-references
    description: Repos useful for LLM context
    color: "#4CAF50"

repos:
  - forge: github
    owner: charmbracelet
    name: bubbletea
    forge_id: "MDEwOlJlcG9zaXRvcnkyNDUyMTQ1MDA="
    collections: [llm-references]
    notes: "TUI framework"
    pinned: true
    tags: [golang, tui]
```

## TUI Interface

### Views

1. **List View** — Filterable table of all repos
2. **Search View** — Query input with live results
3. **Detail View** — Full metadata + README preview
4. **Action Menu** — Context actions for selected repos

### Key Bindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `/` | Focus search |
| `Enter` | Detail (single) / Actions (multi) |
| `Space` | Toggle selection |
| `a` / `A` | Select all / Deselect all |
| `e` | Open in $EDITOR |
| `w` | Open URL in browser |
| `y` | Copy to clipboard |
| `?` | Help |
| `q` | Quit |

## Search Architecture

### Two-Tier Index

1. **BM25 (always on)** — SQLite FTS5 over name, description, topics, README, manifest files
2. **Vector (optional)** — Embeddings for semantic search, reranks BM25 candidates

### Signal Boosting

Results are boosted by configurable signals:

- Star recency (recently starred → higher)
- Push recency (recently active → higher)
- Language match (if filter active)

### High-Signal Files

Beyond README, these files are extracted and indexed:

- `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml` — Dependencies
- `Dockerfile`, `docker-compose.yml` — Container configuration

This enables queries like "repos using sqlite and bubbletea" without full code indexing.

## Data Model

### Directory Layout

```
~/.config/starbase/           # Syncable (dotfiles)
├── config.yaml
└── manifest.yaml

~/.local/share/starbase/      # Machine-local
├── starbase.db
├── cache/
└── clones/
    ├── github/<owner>/<repo>/
    └── gitlab/<host>/<ns>/<repo>/
```

### Database Schema

```sql
-- Identity (stable across renames)
CREATE TABLE repos (
    id INTEGER PRIMARY KEY,
    forge TEXT NOT NULL,
    forge_id TEXT NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    web_url TEXT NOT NULL,
    local_path TEXT,
    starred_at TEXT,
    cloned_at TEXT,
    synced_at TEXT,
    status TEXT DEFAULT 'active',
    UNIQUE(forge, forge_id)
);

-- Mutable metadata
CREATE TABLE repo_metadata (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id),
    description TEXT,
    language TEXT,
    topics TEXT,              -- JSON array
    stars_count INTEGER,
    default_branch TEXT,
    is_archived INTEGER,
    pushed_at TEXT,
    size_kb INTEGER
);

-- Indexed documents
CREATE TABLE repo_documents (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    doc_type TEXT NOT NULL,
    filename TEXT NOT NULL,
    content TEXT,
    content_hash TEXT
);

-- FTS5 index
CREATE VIRTUAL TABLE search_index USING fts5(
    repo_id, name, description, topics,
    readme_content, manifest_content,
    tokenize='porter'
);

-- Local annotations
CREATE TABLE repo_annotations (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id),
    collections TEXT,         -- JSON array
    notes TEXT,
    is_pinned INTEGER DEFAULT 0
);

-- Vector embeddings (optional)
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    doc_type TEXT NOT NULL,
    chunk_index INTEGER,
    embedding BLOB,
    model TEXT
);
```

## Risk Register

| Risk | Mitigation |
|------|------------|
| API rate limits | ETag caching, conditional requests, backoff |
| Disk growth | Clone filters, LFS skip, pruning policies |
| Auth token exposure | OS keyring, never log tokens |
| Repo renames/moves | Stable forge IDs, path migration |
| Accidental unstar | Explicit `--unstar` flag, confirmation |
| TUI complexity | Minimal views, message-driven architecture |
| Embedding latency | Local-first (ollama), hybrid as opt-in |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `STARBASE_CONFIG_DIR` | Override config directory |
| `STARBASE_DATA_DIR` | Override data directory |
| `STARBASE_GITHUB_TOKEN` | GitHub token (fallback) |
| `STARBASE_GITLAB_TOKEN` | GitLab token (fallback) |
| `EDITOR` | Editor for open action |
| `BROWSER` | Browser for URL action |

## Future Considerations

- **MCP Server** — Expose search as tool for LLM agents
- **Watch Mode** — Background sync daemon
- **Plain Git URLs** — Track repos not starred anywhere
- **GitHub Enterprise / Self-hosted GitLab** — Via provider abstraction
