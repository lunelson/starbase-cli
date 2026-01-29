# starbase-cli: Code Mode Handoff

## Project Overview

**starbase-cli** is a Go-based CLI/TUI tool for maintaining a searchable local mirror of GitHub/GitLab starred repositories, optimized for LLM-assisted development workflows.

### Core Value Proposition

Stars are a curation signal that typically becomes an unmanageable inbox. This tool transforms them into a **personal code atlas**:

1. **Local shallow clones** of starred repos (recent window by default)
2. **Fast hybrid search** (BM25 + optional vector) over names, descriptions, tags, READMEs, and high-signal files
3. **TUI interface** (Bubbletea) for browsing, multi-select, and actions (copy paths, open editor, open browser)
4. **LLM workflow integration** — export repo context in formats suitable for feeding to coding agents
5. **Multi-machine sync** — config and manifest sync via dotfiles; database/clones are derived state

### Prior Art

This is an expansion of [lunelson/clones-cli](https://github.com/lunelson/clones-cli). Preserve the mental model of "read-only reference archive" while adding search, TUI, and forge integration.

---

## Critical Workflow Constraints

### 1. Jujutsu VCS Commits

After completing each step (once all tests pass), commit with Jujutsu:

```bash
jj commit -m "phase-{N}.{step}: {description}"
```

Example:
```bash
jj commit -m "phase-1.3: implement GitHub stars API client with pagination"
```

**Do not proceed to the next step until the current step's tests pass and the commit is made.**

### 2. Test-Gated Progress

Each phase has explicit verification gates. The pattern is:

1. Write/update tests for the step
2. Implement the feature
3. Run tests: `go test ./...`
4. If tests pass → `jj commit`
5. If tests fail → fix and re-run

### 3. Test Philosophy

- **Unit tests** for pure functions and data transformations
- **Integration tests** for Git operations (use temp directories)
- **Mock tests** for API clients (use `httptest` or interface mocks)
- **Golden file tests** for CLI output formats
- **Table-driven tests** preferred for Go idiom

---

## Architecture Decisions

### Directory Layout

```
~/.config/starbase/           # SYNCABLE via dotfiles (yadm)
├── config.yaml               # Settings, policies, auth references
└── manifest.yaml             # Source of truth: tracked repos + annotations

~/.local/share/starbase/      # MACHINE-LOCAL, not synced
├── starbase.db               # SQLite: cache + FTS5 + embeddings
├── cache/                    # API response cache (ETags)
└── clones/                   # Shallow clones
    ├── github/<owner>/<repo>/
    └── gitlab/<host>/<namespace>/<repo>/
```

### Key Principle: Manifest as Intent, Database as Cache

The **manifest.yaml** is the source of truth for what repos to track and user annotations. The database and clones are derived state that can be rebuilt on any machine via `starbase sync --full`.

### Database Schema (SQLite + FTS5)

```sql
-- Core identity (stable across renames)
CREATE TABLE repos (
    id INTEGER PRIMARY KEY,
    forge TEXT NOT NULL,           -- 'github' | 'gitlab'
    forge_id TEXT NOT NULL,        -- stable ID from forge
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,       -- owner/name (mutable)
    clone_url TEXT NOT NULL,
    web_url TEXT NOT NULL,
    local_path TEXT,               -- null if not cloned
    starred_at TEXT,               -- ISO8601, nullable
    cloned_at TEXT,                -- ISO8601, nullable
    synced_at TEXT,                -- ISO8601, nullable
    status TEXT DEFAULT 'active',  -- active|archived|unavailable|pending
    UNIQUE(forge, forge_id)
);

-- Mutable metadata (refreshed on sync)
CREATE TABLE repo_metadata (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id),
    description TEXT,
    language TEXT,
    topics TEXT,                   -- JSON array
    stars_count INTEGER,
    forks_count INTEGER,
    default_branch TEXT,
    is_archived INTEGER DEFAULT 0,
    is_private INTEGER DEFAULT 0,
    pushed_at TEXT,                -- last push timestamp
    updated_at TEXT,               -- last update timestamp
    latest_release TEXT,           -- version string
    latest_release_at TEXT,        -- ISO8601
    size_kb INTEGER
);

-- Indexed documents (README + high-signal files)
CREATE TABLE repo_documents (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    doc_type TEXT NOT NULL,        -- 'readme'|'manifest'|'dockerfile'|etc
    filename TEXT NOT NULL,
    content TEXT,
    content_hash TEXT,             -- for change detection
    extracted_at TEXT
);

-- FTS5 virtual table for BM25 search
CREATE VIRTUAL TABLE search_index USING fts5(
    repo_id,
    name,
    description,
    topics,
    readme_content,
    manifest_content,              -- go.mod, package.json, etc.
    content='',                    -- contentless for efficiency
    tokenize='porter'
);

-- Local annotations (from manifest, cached here)
CREATE TABLE repo_annotations (
    repo_id INTEGER PRIMARY KEY REFERENCES repos(id),
    collections TEXT,              -- JSON array of collection names
    notes TEXT,
    is_pinned INTEGER DEFAULT 0,
    local_tags TEXT                -- JSON array
);

-- Optional: vector embeddings
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY,
    repo_id INTEGER REFERENCES repos(id),
    doc_type TEXT NOT NULL,
    chunk_index INTEGER,
    embedding BLOB,                -- float32 array
    model TEXT,
    created_at TEXT
);
```

### Config Schema (config.yaml)

```yaml
# ~/.config/starbase/config.yaml
version: 1

# Base directory for machine-local data
data_dir: ~/.local/share/starbase

# Clone settings
clone:
  depth: 1                        # shallow clone depth
  single_branch: true
  skip_submodules: true
  skip_lfs: true                  # set GIT_LFS_SKIP_SMUDGE=1

# Sync policies
sync:
  default_window: 30d             # only clone stars from last N days on init
  clone_missing: true             # clone newly starred repos on sync
  clone_private: false            # include private repos
  clone_archived: false           # include archived repos
  max_repos_per_sync: 100         # safety valve
  reset_on_conflict: true         # git reset --hard on sync

# Index settings
index:
  readme: true
  high_signal_files:              # additional files to index
    - go.mod
    - package.json
    - Cargo.toml
    - pyproject.toml
    - Dockerfile
    - docker-compose.yml
  max_file_size_kb: 100           # skip files larger than this

# Search settings
search:
  engine: bm25                    # 'bm25' | 'hybrid'
  embedding_model: nomic-embed-text
  embedding_provider: ollama      # 'ollama' | 'openai' | 'local'
  default_limit: 20

# Signal boosts for ranking (weights)
ranking:
  star_recency_weight: 0.2
  push_recency_weight: 0.1
  language_match_weight: 0.15

# Forges
forges:
  github:
    enabled: true
    # Token loaded from: keyring > env (STARBASE_GITHUB_TOKEN) > gh CLI
  gitlab:
    enabled: false
    host: gitlab.com              # for self-hosted

# Export defaults
export:
  default_format: paths           # paths | json | markdown | yaml
  include_readme: false

# Pruning policies
prune:
  max_total_size_gb: 50
  max_repo_age_days: 365
  keep_pinned: true
```

### Manifest Schema (manifest.yaml)

```yaml
# ~/.config/starbase/manifest.yaml
version: 1

# Collections (local-only groupings)
collections:
  - name: llm-references
    description: Repos useful for LLM context
    color: "#4CAF50"
  - name: studying
    description: Currently learning from these
    color: "#2196F3"

# Tracked repositories
repos:
  - forge: github
    owner: charmbracelet
    name: bubbletea
    forge_id: "MDEwOlJlcG9zaXRvcnkyNDUyMTQ1MDA="  # stable GitHub node ID
    collections:
      - llm-references
    notes: "TUI framework for starbase"
    pinned: true
    tags:
      - golang
      - tui

  - forge: github
    owner: anthropics
    name: anthropic-cookbook
    forge_id: "R_kgDOKxxx"
    collections:
      - llm-references
      - studying
    notes: "Reference implementations for Claude"
```

---

## Commands Specification

```
starbase
├── init                          # Initialize starbase
├── auth
│   ├── login [--forge=github]    # Authenticate with forge
│   ├── status                    # Show auth status
│   └── logout [--forge=github]   # Clear credentials
├── sync
│   ├── [--full]                  # Clone all, not just recent window
│   ├── [--metadata-only]         # Skip git operations
│   ├── [--pull-only]             # Update existing clones only
│   ├── [--since=DURATION]        # Override recency window
│   └── [--dry-run]               # Show plan without executing
├── add <url>                     # Clone + star + add to manifest
│   └── [--no-star]               # Don't star on forge
├── remove <query>                # Remove local clone
│   ├── [--unstar]                # Also unstar on forge
│   └── [--yes]                   # Skip confirmation
├── search <query>                # Search repos
│   ├── [--limit=N]
│   ├── [--json|--markdown|--yaml]
│   └── [--field=FIELD]           # lang:, topic:, has:, collection:
├── list                          # List all tracked repos
│   ├── [--format=FORMAT]
│   ├── [--collection=NAME]
│   └── [--status=STATUS]
├── export <query>                # Export repo info
│   ├── [--format=FORMAT]         # paths|json|markdown|yaml|workspace
│   ├── [--include-readme]
│   └── [--output=PATH]
├── collection
│   ├── list
│   ├── create <name>
│   ├── delete <name>
│   ├── add <collection> <query>
│   └── remove <collection> <query>
├── note <query> <text>           # Add/update note
├── pin <query>                   # Pin repo (prevent pruning)
├── unpin <query>
├── prune                         # Remove clones per policy
│   ├── [--max-disk=SIZE]
│   ├── [--older-than=DURATION]
│   ├── [--dry-run]
│   └── [--keep-pinned]
├── index
│   ├── rebuild                   # Rebuild FTS index
│   └── rebuild-embeddings        # Rebuild vector embeddings
├── browse                        # Launch TUI
└── serve                         # Start MCP server (future)
    └── [--port=PORT]
```

---

## Implementation Phases

### Phase 1: Foundation + Manifest Architecture

**Goal:** Syncable config/manifest, basic clone management, GitHub stars

#### Step 1.1: Project Scaffold

- [ ] Initialize Go module: `go mod init github.com/lunelson/starbase-cli`
- [ ] Set up directory structure:
  ```
  cmd/starbase/main.go
  internal/
    config/
    manifest/
    database/
    forge/
    git/
    cli/
  pkg/  (if any public APIs)
  testdata/
  ```
- [ ] Add cobra for CLI framework
- [ ] Add koanf for config loading
- [ ] Create Makefile with: `build`, `test`, `lint`, `install`
- [ ] Set up golangci-lint config

**Tests:**
- `go build ./...` succeeds
- `go test ./...` passes (empty but runnable)
- `make lint` passes

**Commit:** `jj commit -m "phase-1.1: project scaffold with cobra, koanf, makefile"`

---

#### Step 1.2: Config + Manifest Loading

- [ ] Implement `internal/config/config.go`:
  - Load from `~/.config/starbase/config.yaml`
  - Merge with defaults
  - Environment variable overrides
  - Validation
- [ ] Implement `internal/manifest/manifest.go`:
  - Load/save `~/.config/starbase/manifest.yaml`
  - CRUD operations for repos and collections
  - Validation
- [ ] Implement `init` command:
  - Create directories
  - Write default config.yaml
  - Write empty manifest.yaml

**Tests:**
```go
// config_test.go
func TestLoadConfigDefaults(t *testing.T)
func TestLoadConfigFromFile(t *testing.T)
func TestConfigEnvOverrides(t *testing.T)
func TestConfigValidation(t *testing.T)

// manifest_test.go
func TestLoadEmptyManifest(t *testing.T)
func TestManifestAddRepo(t *testing.T)
func TestManifestRemoveRepo(t *testing.T)
func TestManifestCollectionCRUD(t *testing.T)
func TestManifestSaveLoad(t *testing.T)
```

**Verification Gate:**
```bash
go test ./internal/config/... ./internal/manifest/... -v
```

**Commit:** `jj commit -m "phase-1.2: config and manifest loading with validation"`

---

#### Step 1.3: SQLite Database Layer

- [ ] Implement `internal/database/database.go`:
  - Open/create database at configured path
  - Run migrations (embed SQL files)
  - CRUD for repos, metadata, documents
- [ ] Implement `internal/database/migrations/`:
  - `001_initial_schema.sql`
- [ ] Add FTS5 virtual table creation

**Tests:**
```go
func TestDatabaseCreate(t *testing.T)
func TestDatabaseMigrations(t *testing.T)
func TestReposCRUD(t *testing.T)
func TestRepoMetadataCRUD(t *testing.T)
func TestDocumentsCRUD(t *testing.T)
func TestFTS5IndexPopulation(t *testing.T)
```

**Verification Gate:**
```bash
go test ./internal/database/... -v
```

**Commit:** `jj commit -m "phase-1.3: SQLite database layer with migrations and FTS5"`

---

#### Step 1.4: GitHub API Client

- [ ] Implement `internal/forge/forge.go`:
  - `Forge` interface definition
  - `StarredRepo` struct (normalized)
- [ ] Implement `internal/forge/github/github.go`:
  - Authenticate (keyring → env → gh CLI fallback)
  - List starred repos with `starred_at` (requires `Accept: application/vnd.github.star+json`)
  - Handle pagination
  - Rate limiting with backoff
  - ETag caching

**Tests:**
```go
// Use httptest for mock server
func TestGitHubListStars(t *testing.T)
func TestGitHubPagination(t *testing.T)
func TestGitHubRateLimitBackoff(t *testing.T)
func TestGitHubStarredAtParsing(t *testing.T)
func TestGitHubAuthFallback(t *testing.T)
```

**Verification Gate:**
```bash
go test ./internal/forge/... -v
# Optional: integration test with real API (skipped in CI)
go test ./internal/forge/... -v -tags=integration
```

**Commit:** `jj commit -m "phase-1.4: GitHub API client with pagination and rate limiting"`

---

#### Step 1.5: Git Operations

- [ ] Implement `internal/git/git.go`:
  - Shallow clone (single branch, configurable depth)
  - Clone with LFS skip (`GIT_LFS_SKIP_SMUDGE=1`)
  - Clone without submodules
  - Fetch + reset to origin HEAD
  - Detect current HEAD SHA
  - Get default branch from remote

**Tests:**
```go
// Use temp directories with real git operations
func TestShallowClone(t *testing.T)
func TestCloneSkipsLFS(t *testing.T)
func TestCloneSkipsSubmodules(t *testing.T)
func TestFetchAndReset(t *testing.T)
func TestDetectDefaultBranch(t *testing.T)
```

**Verification Gate:**
```bash
go test ./internal/git/... -v
```

**Commit:** `jj commit -m "phase-1.5: git operations for shallow clone and sync"`

---

#### Step 1.6: Sync Command (Core)

- [ ] Implement `sync` command:
  - Fetch stars from GitHub
  - Apply recency window filter
  - Update manifest with new stars
  - Clone missing repos (within window)
  - Update existing clones (fetch + reset)
  - Update database with metadata
  - Handle `--dry-run`, `--full`, `--metadata-only`, `--pull-only`, `--since`

**Tests:**
```go
func TestSyncDryRun(t *testing.T)
func TestSyncRecencyWindow(t *testing.T)
func TestSyncMetadataOnly(t *testing.T)
func TestSyncPullOnly(t *testing.T)
func TestSyncFull(t *testing.T)
func TestSyncHandlesDeletedRepos(t *testing.T)
func TestSyncHandlesRenamedRepos(t *testing.T)
```

**Verification Gate:**
```bash
go test ./internal/cli/... -v -run TestSync
# End-to-end test with mock forge
go test ./... -v -tags=e2e
```

**Commit:** `jj commit -m "phase-1.6: sync command with recency window and dry-run"`

---

#### Step 1.7: List Command

- [ ] Implement `list` command:
  - List all tracked repos from database
  - Filter by status, collection
  - Output formats: table (default), json, paths

**Tests:**
```go
func TestListDefault(t *testing.T)
func TestListFilterByStatus(t *testing.T)
func TestListFilterByCollection(t *testing.T)
func TestListJSONOutput(t *testing.T)
func TestListPathsOutput(t *testing.T)
```

**Golden file tests for output formats.**

**Verification Gate:**
```bash
go test ./internal/cli/... -v -run TestList
```

**Commit:** `jj commit -m "phase-1.7: list command with filters and output formats"`

---

### Phase 1 Completion Gate

**Full test suite must pass:**
```bash
go test ./... -v
make lint
```

**Manual verification:**
```bash
starbase init
starbase sync --dry-run
starbase sync --since=7d
starbase list
starbase list --json
```

**Commit:** `jj commit -m "phase-1: foundation complete - manifest, config, GitHub sync, list"`

---

### Phase 2: Search Index (BM25 + Signals)

#### Step 2.1: README Extraction

- [ ] Implement `internal/index/extractor.go`:
  - Find README file (case-insensitive, multiple extensions)
  - Extract text content (strip markdown formatting or keep raw)
  - Handle missing READMEs gracefully

**Tests:**
```go
func TestFindReadme(t *testing.T)          // README.md, readme.md, README, etc.
func TestExtractReadmeContent(t *testing.T)
func TestMissingReadmeHandling(t *testing.T)
```

**Commit:** `jj commit -m "phase-2.1: README extraction from clones"`

---

#### Step 2.2: High-Signal File Extraction

- [ ] Implement extraction for: `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `Dockerfile`, `docker-compose.yml`
- [ ] Configurable file list from config
- [ ] Size limit enforcement

**Tests:**
```go
func TestExtractGoMod(t *testing.T)
func TestExtractPackageJson(t *testing.T)
func TestExtractDockerfile(t *testing.T)
func TestFileSizeLimit(t *testing.T)
func TestConfigurableFileList(t *testing.T)
```

**Commit:** `jj commit -m "phase-2.2: high-signal file extraction (go.mod, package.json, etc)"`

---

#### Step 2.3: FTS5 Index Population

- [ ] Implement `internal/index/indexer.go`:
  - Populate `repo_documents` table
  - Populate FTS5 `search_index`
  - Incremental update (only changed content via hash)
  - Full rebuild command

**Tests:**
```go
func TestIndexPopulation(t *testing.T)
func TestIncrementalIndexUpdate(t *testing.T)
func TestIndexRebuild(t *testing.T)
func TestContentHashChangeDetection(t *testing.T)
```

**Commit:** `jj commit -m "phase-2.3: FTS5 index population with incremental updates"`

---

#### Step 2.4: BM25 Search Implementation

- [ ] Implement `internal/search/search.go`:
  - Basic BM25 query via FTS5
  - Field-specific queries: `lang:go`, `topic:cli`, `has:dockerfile`
  - Query parsing
  - Result ranking

**Tests:**
```go
func TestBasicSearch(t *testing.T)
func TestSearchByLanguage(t *testing.T)
func TestSearchByTopic(t *testing.T)
func TestSearchHasDockerfile(t *testing.T)
func TestSearchQueryParsing(t *testing.T)
func TestSearchRanking(t *testing.T)
```

**Commit:** `jj commit -m "phase-2.4: BM25 search with field-specific queries"`

---

#### Step 2.5: Signal Boosting

- [ ] Implement ranking signals:
  - Star recency boost
  - Push recency boost
  - Language match boost (if filter active)
- [ ] Configurable weights from config.yaml

**Tests:**
```go
func TestStarRecencyBoost(t *testing.T)
func TestPushRecencyBoost(t *testing.T)
func TestLanguageMatchBoost(t *testing.T)
func TestConfigurableWeights(t *testing.T)
```

**Commit:** `jj commit -m "phase-2.5: signal boosting for search ranking"`

---

#### Step 2.6: Search + Export Commands

- [ ] Implement `search` command:
  - Query argument
  - `--limit`, `--json`, `--markdown`, `--yaml`
  - Display results with relevance scores
- [ ] Implement `export` command:
  - Same query syntax as search
  - Output formats: paths, json, markdown, yaml
  - `--include-readme` option
  - `--output` for file output

**Tests:**
```go
func TestSearchCommand(t *testing.T)
func TestSearchLimit(t *testing.T)
func TestSearchJSONOutput(t *testing.T)
func TestExportPaths(t *testing.T)
func TestExportMarkdown(t *testing.T)
func TestExportIncludeReadme(t *testing.T)
func TestExportToFile(t *testing.T)
```

**Golden file tests for all output formats.**

**Commit:** `jj commit -m "phase-2.6: search and export commands"`

---

### Phase 2 Completion Gate

```bash
go test ./... -v
make lint
```

**Manual verification:**
```bash
starbase sync
starbase search "bubbletea tui"
starbase search "lang:go topic:cli"
starbase search "has:dockerfile"
starbase export "sqlite" --format=markdown
```

**Commit:** `jj commit -m "phase-2: search index complete - BM25, signals, export"`

---

### Phase 3: Collections + Annotations

#### Step 3.1: Collection Management

- [ ] Implement `collection` subcommands:
  - `list`, `create`, `delete`
  - `add <collection> <query>` - add repos matching query to collection
  - `remove <collection> <query>`
- [ ] Update manifest on changes

**Tests:**
```go
func TestCollectionCreate(t *testing.T)
func TestCollectionDelete(t *testing.T)
func TestCollectionAddRepos(t *testing.T)
func TestCollectionRemoveRepos(t *testing.T)
func TestCollectionPersistsToManifest(t *testing.T)
```

**Commit:** `jj commit -m "phase-3.1: collection management commands"`

---

#### Step 3.2: Notes and Pins

- [ ] Implement `note <query> <text>` command
- [ ] Implement `pin` / `unpin` commands
- [ ] Update manifest and database

**Tests:**
```go
func TestAddNote(t *testing.T)
func TestUpdateNote(t *testing.T)
func TestPinRepo(t *testing.T)
func TestUnpinRepo(t *testing.T)
func TestPinPersistsToManifest(t *testing.T)
```

**Commit:** `jj commit -m "phase-3.2: notes and pin commands"`

---

#### Step 3.3: Collection-Scoped Search

- [ ] Add `collection:NAME` filter to search
- [ ] Update search command to support collection filtering

**Tests:**
```go
func TestSearchByCollection(t *testing.T)
func TestSearchMultipleCollections(t *testing.T)
```

**Commit:** `jj commit -m "phase-3.3: collection-scoped search"`

---

### Phase 3 Completion Gate

```bash
go test ./... -v
```

**Manual verification:**
```bash
starbase collection create llm-refs
starbase collection add llm-refs "lang:go"
starbase search "collection:llm-refs"
starbase note bubbletea "TUI framework for this project"
starbase pin bubbletea
```

**Commit:** `jj commit -m "phase-3: collections and annotations complete"`

---

### Phase 4: TUI Core

#### Step 4.1: Bubbletea Scaffold

- [ ] Set up `internal/tui/` package
- [ ] Create main model with view states: list, search, detail
- [ ] Implement basic message loop
- [ ] Add lipgloss styling

**Tests:**
```go
// TUI tests use the tea.Test helpers
func TestTUIInitialization(t *testing.T)
func TestTUIViewSwitching(t *testing.T)
func TestTUIQuit(t *testing.T)
```

**Commit:** `jj commit -m "phase-4.1: Bubbletea scaffold with view states"`

---

#### Step 4.2: List View

- [ ] Implement list view with bubbles/list or bubbles/table
- [ ] Virtual scrolling for large lists
- [ ] Display: name, description (truncated), language, star date
- [ ] Collection filtering via keyboard

**Tests:**
```go
func TestListViewRendering(t *testing.T)
func TestListViewScrolling(t *testing.T)
func TestListViewFiltering(t *testing.T)
```

**Commit:** `jj commit -m "phase-4.2: list view with virtual scrolling"`

---

#### Step 4.3: Search View

- [ ] Implement search input with bubbles/textinput
- [ ] Live filtering as user types (debounced)
- [ ] Results display below input
- [ ] Relevance score display

**Tests:**
```go
func TestSearchViewInput(t *testing.T)
func TestSearchViewLiveFiltering(t *testing.T)
func TestSearchViewDebounce(t *testing.T)
```

**Commit:** `jj commit -m "phase-4.3: search view with live filtering"`

---

#### Step 4.4: Detail View

- [ ] Implement detail view for selected repo
- [ ] Show: full metadata, notes, collections, README preview
- [ ] Scrollable README content
- [ ] Return to list with escape/backspace

**Tests:**
```go
func TestDetailViewRendering(t *testing.T)
func TestDetailViewReadmeScroll(t *testing.T)
func TestDetailViewNavigation(t *testing.T)
```

**Commit:** `jj commit -m "phase-4.4: detail view with README preview"`

---

#### Step 4.5: Multi-Select Mode

- [ ] Space to toggle selection
- [ ] Visual indicator for selected items
- [ ] Selection count in status bar
- [ ] Select all / deselect all shortcuts

**Tests:**
```go
func TestMultiSelectToggle(t *testing.T)
func TestMultiSelectVisualIndicator(t *testing.T)
func TestSelectAll(t *testing.T)
func TestDeselectAll(t *testing.T)
```

**Commit:** `jj commit -m "phase-4.5: multi-select mode"`

---

### Phase 4 Completion Gate

```bash
go test ./... -v
```

**Manual verification:**
```bash
starbase browse
# Navigate with j/k or arrows
# Press / to search
# Press Enter for detail view
# Press Space to multi-select
# Press q to quit
```

**Commit:** `jj commit -m "phase-4: TUI core complete - list, search, detail, multi-select"`

---

### Phase 5: TUI Actions + Polish

#### Step 5.1: Action Menu

- [ ] Implement action menu (appears on Enter with selection)
- [ ] Actions: copy paths, copy URLs, open in editor, open in browser
- [ ] Execute actions on selected repos

**Tests:**
```go
func TestActionMenuDisplay(t *testing.T)
func TestActionCopyPaths(t *testing.T)
func TestActionOpenEditor(t *testing.T)
func TestActionOpenBrowser(t *testing.T)
```

**Commit:** `jj commit -m "phase-5.1: action menu with copy, editor, browser actions"`

---

#### Step 5.2: Clipboard Integration

- [ ] Implement clipboard copy (use golang.design/x/clipboard or atotto/clipboard)
- [ ] Format selection for clipboard: paths, URLs, markdown

**Tests:**
```go
func TestClipboardCopyPaths(t *testing.T)
func TestClipboardCopyMarkdown(t *testing.T)
```

**Commit:** `jj commit -m "phase-5.2: clipboard integration"`

---

#### Step 5.3: Progress Indicators

- [ ] Implement progress display for sync operations
- [ ] Show: current repo, progress bar, ETA
- [ ] Allow cancellation (Ctrl+C graceful)

**Tests:**
```go
func TestProgressDisplay(t *testing.T)
func TestProgressCancellation(t *testing.T)
```

**Commit:** `jj commit -m "phase-5.3: progress indicators for sync"`

---

#### Step 5.4: Status Bar + Help

- [ ] Persistent status bar: selection count, total repos, sync status
- [ ] Help overlay (?) with keybindings
- [ ] Error display in status bar

**Tests:**
```go
func TestStatusBarDisplay(t *testing.T)
func TestHelpOverlay(t *testing.T)
func TestErrorDisplay(t *testing.T)
```

**Commit:** `jj commit -m "phase-5.4: status bar and help overlay"`

---

### Phase 5 Completion Gate

```bash
go test ./... -v
```

**Manual verification:**
- Full TUI workflow: browse → search → select → action
- Copy paths to clipboard, verify
- Open in editor, verify
- Trigger sync from TUI, verify progress

**Commit:** `jj commit -m "phase-5: TUI actions complete - full interactive workflow"`

---

### Phase 6: Hybrid Search (Vector)

#### Step 6.1: Embedding Generation

- [ ] Implement `internal/embedding/embedding.go`:
  - Interface for embedding providers
  - Ollama implementation (nomic-embed-text)
  - Chunking strategy for long documents

**Tests:**
```go
func TestOllamaEmbedding(t *testing.T)
func TestChunkingStrategy(t *testing.T)
func TestEmbeddingDimensions(t *testing.T)
```

**Commit:** `jj commit -m "phase-6.1: embedding generation with Ollama"`

---

#### Step 6.2: Embeddings Storage

- [ ] Implement embeddings table CRUD
- [ ] Store/retrieve embeddings efficiently
- [ ] Track model version for invalidation

**Tests:**
```go
func TestEmbeddingsStore(t *testing.T)
func TestEmbeddingsRetrieve(t *testing.T)
func TestEmbeddingsModelVersioning(t *testing.T)
```

**Commit:** `jj commit -m "phase-6.2: embeddings storage in SQLite"`

---

#### Step 6.3: Vector Search

- [ ] Implement cosine similarity search
- [ ] Query embedding generation
- [ ] Top-K retrieval

**Tests:**
```go
func TestVectorSearch(t *testing.T)
func TestCosineSimilarity(t *testing.T)
func TestTopKRetrieval(t *testing.T)
```

**Commit:** `jj commit -m "phase-6.3: vector search implementation"`

---

#### Step 6.4: Hybrid Search Integration

- [ ] Implement hybrid retrieval:
  1. BM25 to get top N candidates
  2. Vector rerank top K
- [ ] Configurable via `search.engine: hybrid`
- [ ] Graceful fallback if embeddings unavailable

**Tests:**
```go
func TestHybridSearch(t *testing.T)
func TestHybridFallbackToBM25(t *testing.T)
func TestHybridConfigurable(t *testing.T)
```

**Commit:** `jj commit -m "phase-6.4: hybrid search - BM25 + vector rerank"`

---

#### Step 6.5: Index Rebuild Commands

- [ ] Implement `index rebuild` command
- [ ] Implement `index rebuild-embeddings` command
- [ ] Progress display for embedding generation

**Tests:**
```go
func TestIndexRebuildCommand(t *testing.T)
func TestEmbeddingsRebuildCommand(t *testing.T)
```

**Commit:** `jj commit -m "phase-6.5: index rebuild commands"`

---

### Phase 6 Completion Gate

```bash
go test ./... -v
```

**Manual verification:**
```bash
# Ensure ollama is running with nomic-embed-text
starbase index rebuild-embeddings
starbase search "machine learning framework" --engine=hybrid
```

**Commit:** `jj commit -m "phase-6: hybrid search complete - BM25 + vector"`

---

### Phase 7: GitLab + Provider Abstraction

#### Step 7.1: Provider Interface Refinement

- [ ] Ensure `Forge` interface covers all operations:
  - ListStars, Star, Unstar
  - GetRepo, GetReadme
  - Authentication

**Tests:**
```go
func TestForgeInterfaceCompleteness(t *testing.T)
```

**Commit:** `jj commit -m "phase-7.1: forge interface refinement"`

---

#### Step 7.2: GitLab Client

- [ ] Implement `internal/forge/gitlab/gitlab.go`:
  - List starred projects
  - Handle pagination
  - Support self-hosted instances

**Tests:**
```go
func TestGitLabListStars(t *testing.T)
func TestGitLabPagination(t *testing.T)
func TestGitLabSelfHosted(t *testing.T)
```

**Commit:** `jj commit -m "phase-7.2: GitLab API client"`

---

#### Step 7.3: Add + Remove Commands

- [ ] Implement `add <url>`:
  - Clone repo
  - Star on forge (unless `--no-star`)
  - Add to manifest
- [ ] Implement `remove <query>`:
  - Remove local clone
  - Optionally unstar (`--unstar`)
  - Confirmation required (or `--yes`)

**Tests:**
```go
func TestAddCommand(t *testing.T)
func TestAddNoStar(t *testing.T)
func TestRemoveCommand(t *testing.T)
func TestRemoveWithUnstar(t *testing.T)
func TestRemoveConfirmation(t *testing.T)
```

**Commit:** `jj commit -m "phase-7.3: add and remove commands with forge integration"`

---

### Phase 7 Completion Gate

```bash
go test ./... -v
```

**Manual verification:**
```bash
starbase add https://github.com/some/repo
starbase add https://gitlab.com/some/project
starbase remove some-repo --unstar --yes
starbase sync  # should work with both forges
```

**Commit:** `jj commit -m "phase-7: GitLab support complete - multi-forge"`

---

### Phase 8: Advanced Features

#### Step 8.1: Pruning

- [ ] Implement `prune` command:
  - `--max-disk`: remove oldest until under limit
  - `--older-than`: remove repos not synced in N days
  - `--keep-pinned`: never prune pinned repos
  - `--dry-run`: show what would be pruned

**Tests:**
```go
func TestPruneMaxDisk(t *testing.T)
func TestPruneOlderThan(t *testing.T)
func TestPruneKeepsPinned(t *testing.T)
func TestPruneDryRun(t *testing.T)
```

**Commit:** `jj commit -m "phase-8.1: prune command"`

---

#### Step 8.2: Workspace Export

- [ ] Implement `export --format=workspace`:
  - Copy selected repos to target directory
  - Generate `CONTEXT.md` with repo list and descriptions
  - Optionally include READMEs inline

**Tests:**
```go
func TestWorkspaceExport(t *testing.T)
func TestWorkspaceContextMd(t *testing.T)
```

**Commit:** `jj commit -m "phase-8.2: workspace export for LLM context"`

---

#### Step 8.3: Auth Commands

- [ ] Implement `auth login`:
  - GitHub: device flow or token input
  - GitLab: token input
  - Store in OS keyring
- [ ] Implement `auth status`: show auth state per forge
- [ ] Implement `auth logout`: clear credentials

**Tests:**
```go
func TestAuthLogin(t *testing.T)
func TestAuthStatus(t *testing.T)
func TestAuthLogout(t *testing.T)
func TestAuthKeyringStorage(t *testing.T)
```

**Commit:** `jj commit -m "phase-8.3: auth commands with keyring storage"`

---

#### Step 8.4: MCP Server (Stretch)

- [ ] Implement `serve` command
- [ ] Expose tools: `search_stars`, `get_readme`, `list_by_language`
- [ ] MCP protocol compliance

**Tests:**
```go
func TestMCPServerStart(t *testing.T)
func TestMCPSearchTool(t *testing.T)
func TestMCPGetReadmeTool(t *testing.T)
```

**Commit:** `jj commit -m "phase-8.4: MCP server mode"`

---

### Phase 8 Completion Gate

```bash
go test ./... -v
make lint
```

**Full manual verification of all features.**

**Commit:** `jj commit -m "phase-8: advanced features complete - prune, workspace, auth, MCP"`

---

## Final Checklist

- [ ] All tests pass: `go test ./... -v`
- [ ] Linting passes: `make lint`
- [ ] Documentation: README.md with usage examples
- [ ] Installation: `make install` works
- [ ] Release: tagged version, binaries built

**Final commit:** `jj commit -m "v1.0.0: starbase-cli initial release"`

---

## Appendix: Test Utilities

### Common Test Helpers

```go
// internal/testutil/testutil.go

package testutil

import (
    "os"
    "path/filepath"
    "testing"
)

// TempStarbase creates a temporary starbase directory for testing
func TempStarbase(t *testing.T) (configDir, dataDir string, cleanup func()) {
    t.Helper()
    
    configDir, err := os.MkdirTemp("", "starbase-config-*")
    if err != nil {
        t.Fatal(err)
    }
    
    dataDir, err = os.MkdirTemp("", "starbase-data-*")
    if err != nil {
        os.RemoveAll(configDir)
        t.Fatal(err)
    }
    
    cleanup = func() {
        os.RemoveAll(configDir)
        os.RemoveAll(dataDir)
    }
    
    return configDir, dataDir, cleanup
}

// GitRepo creates a temporary git repository for testing
func GitRepo(t *testing.T, files map[string]string) (path string, cleanup func()) {
    t.Helper()
    
    dir, err := os.MkdirTemp("", "starbase-git-*")
    if err != nil {
        t.Fatal(err)
    }
    
    // git init
    runGit(t, dir, "init")
    runGit(t, dir, "config", "user.email", "test@test.com")
    runGit(t, dir, "config", "user.name", "Test")
    
    // Create files
    for name, content := range files {
        path := filepath.Join(dir, name)
        os.MkdirAll(filepath.Dir(path), 0755)
        os.WriteFile(path, []byte(content), 0644)
    }
    
    // Commit
    runGit(t, dir, "add", ".")
    runGit(t, dir, "commit", "-m", "initial")
    
    cleanup = func() {
        os.RemoveAll(dir)
    }
    
    return dir, cleanup
}
```

### Mock Forge Server

```go
// internal/forge/mock/mock.go

package mock

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
)

type MockForge struct {
    Server *httptest.Server
    Stars  []StarredRepo
}

func NewMockForge(stars []StarredRepo) *MockForge {
    m := &MockForge{Stars: stars}
    m.Server = httptest.NewServer(http.HandlerFunc(m.handler))
    return m
}

func (m *MockForge) handler(w http.ResponseWriter, r *http.Request) {
    switch r.URL.Path {
    case "/user/starred":
        json.NewEncoder(w).Encode(m.Stars)
    default:
        w.WriteHeader(404)
    }
}

func (m *MockForge) Close() {
    m.Server.Close()
}
```

---

## Quick Reference: Key Keybindings (TUI)

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `/` | Focus search |
| `Enter` | Detail view (single) / Action menu (multi) |
| `Space` | Toggle selection |
| `a` | Select all |
| `A` | Deselect all |
| `e` | Open in $EDITOR |
| `w` | Open URL in browser |
| `y` | Copy paths to clipboard |
| `?` | Help |
| `q` / `Esc` | Back / Quit |

---

## Quick Reference: Environment Variables

| Variable | Description |
|----------|-------------|
| `STARBASE_CONFIG_DIR` | Override config directory |
| `STARBASE_DATA_DIR` | Override data directory |
| `STARBASE_GITHUB_TOKEN` | GitHub personal access token |
| `STARBASE_GITLAB_TOKEN` | GitLab personal access token |
| `EDITOR` | Editor for `open in editor` action |
| `BROWSER` | Browser for `open URL` action |
