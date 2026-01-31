# starbase-cli: Development Plan

> Last updated: 2026-01-31

## Current Status

**Phases Complete:**
- ✅ Phase 1: Foundation (config, manifest, SQLite+FTS5, init)
- ✅ Phase 2: GitHub integration (forge interface, API client, git ops, sync)
- ✅ Phase 3: Search (FTS5 BM25, search/list commands, document extraction)
- ✅ Phase 4: Export (export command, collections, notes, pins)
- ✅ Phase 5.1: TUI basics (Bubbletea browse with list/search)
- ✅ Phase 5.2: Multi-select (space/a/n keys)
- ✅ Phase 5.3: Detail view (Enter shows metadata + README with Glamour)
- ✅ Phase 5.4: Actions (e/w/y for editor, browser, clipboard)
- ✅ Phase 5.5: Help overlay (? shows keybindings)
- ✅ Phase 7.3: repo add/remove commands
- ✅ Phase 8.1: Prune command
- ✅ Sync progress feedback (streaming results, spinner, pagination progress)
- ✅ Dev build header with timestamp
- ✅ Top-level `add` and `rm` commands with GitHub star/unstar API

**All tests passing.** Binary builds successfully.

---

## Next Up: Repository Identity & Command Redesign

### Design Goals (from clones-cli patterns)

Reference project: `~/Code/lunelson/clones-cli` (TypeScript predecessor)

#### 1. Multi-Forge Repository ID Format

Current: Uses GitHub's `ForgeID` (node ID like `R_kgDOABC123`)

**Proposed:** `host:owner/repo` format
- `github.com:charmbracelet/bubbletea`
- `gitlab.com:some-org/some-repo`
- `git.company.com:team/internal-tool`

This enables:
- Multiple forges without ID collision
- Human-readable identifiers
- Easy parsing and display

#### 2. Flexible URL Input for `add` Command

Accept multiple URL formats, normalize to extract `host/owner/repo`:

| Input Format | Example |
|--------------|---------|
| HTTPS URL | `https://github.com/owner/repo` |
| SSH URL | `git@github.com:owner/repo.git` |
| Web UI URL | `https://github.com/owner/repo/tree/main/src/file.ts` |
| Short form | `owner/repo` (assumes github.com) |

**Normalization approach:**
1. Parse as URL object (strips anchors, query params)
2. Extract path segments: first = owner, second = repo
3. Generate canonical clone URL

#### 3. `rm` Command Semantics

Four-way matrix of options:

| Action | Flag | Description |
|--------|------|-------------|
| Remove from DB | (default) | Always happens |
| Delete local clone | `--delete` or `-d` | Remove files from disk |
| Keep starred on GitHub | (default) | Star relationship preserved |
| Unstar on GitHub | `--unstar` | Remove star via API |

Examples:
```bash
starbase rm owner/repo              # Remove from DB only, keep files + star
starbase rm owner/repo --delete     # Remove from DB + delete files, keep star
starbase rm owner/repo --unstar     # Remove from DB + unstar, keep files
starbase rm owner/repo -d --unstar  # Full removal: DB, files, and star
```

#### 4. Tombstones Mechanism

**Purpose:** Prevent re-syncing of intentionally removed repos.

When a repo is removed via `rm`:
1. Add its ID to a tombstones list (in manifest or DB)
2. During `sync`, skip any starred repo whose ID is in tombstones
3. Only way to un-tombstone: explicit `add` command

**Storage options:**
- Manifest YAML: `tombstones: ["github.com:owner/repo", ...]`
- Database table: `tombstones (id TEXT PRIMARY KEY, tombstoned_at DATETIME)`

The manifest approach syncs across machines (via dotfiles), while DB is local-only.

**Recommendation:** Store in manifest for cross-machine sync.

### Implementation Steps

| Step | Task |
|------|------|
| 1 | Add URL parser utility (`internal/forge/urlparser.go`) |
| 2 | Update repo ID format in database schema |
| 3 | Migrate existing repos to new ID format |
| 4 | Update `add` command to accept URLs |
| 5 | Update `rm` command with `--delete` and `--unstar` flags |
| 6 | Add tombstones field to manifest |
| 7 | Update sync to respect tombstones |
| 8 | Add `rm` interactive mode (multiselect when no arg)

---

## TUI Architecture

The TUI follows the Charmbracelet ecosystem patterns:

```
internal/tui/
├── model.go   # Main model with viewState enum, Update/View
├── keys.go    # Centralized keyMap with help text
└── styles.go  # Lipgloss styles for all views
```

**View States:**
- `listView` — Main repo list with multi-select
- `searchView` — FTS5 search input
- `detailView` — Full metadata + README (Glamour-rendered)
- `helpView` — Keybindings overlay

**Key Bindings:**
| Key | Action |
|-----|--------|
| `j/↓` `k/↑` | Navigate |
| `space` | Toggle selection |
| `a` / `n` | Select all / none |
| `/` | Search |
| `Enter` | View details |
| `Esc` | Back |
| `?` | Help overlay |
| `q` | Quit |

---

## Remaining Phases

### Phase 6: Hybrid Search (Vector Embeddings)

| Step | Feature |
|------|---------|
| 6.1 | Ollama/OpenAI integration |
| 6.2 | Embeddings storage in SQLite |
| 6.3 | Cosine similarity search |
| 6.4 | Hybrid retrieval (BM25 + rerank) |
| 6.5 | `index rebuild-embeddings` command |

**Requires:** Ollama running with `nomic-embed-text` model (or OpenAI API key)

### Phase 7: GitLab + Multi-Forge

| Step | Feature |
|------|---------|
| 7.1 | Forge interface refinement |
| 7.2 | GitLab API client |

### Phase 8: Advanced Features

| Step | Feature |
|------|---------|
| 8.2 | Workspace export (copy repos + CONTEXT.md) |
| 8.3 | Auth commands (device flow, keyring) |
| 8.4 | MCP server mode (stretch) |

---

## Testing Strategy

### Unit Tests
- `internal/git/pool_test.go` — Worker pool logic
- `internal/tui/*_test.go` — TUI model updates

### Integration Tests
- `internal/sync/sync_test.go` — Full sync with mock forge
- `cmd/starbase/*_test.go` — CLI command tests

### Manual Testing Checklist

After each phase, verify:

```bash
# Core functionality
starbase init                    # Creates ~/.config/starbase/, ~/.local/share/starbase/
starbase sync --since=30d        # Fetches stars, clones repos
starbase search "test query"     # Returns ranked results
starbase list                    # Shows all repos
starbase browse                  # TUI launches

# TUI interactions
# - Navigate with j/k
# - Select with space, a, n
# - Enter for detail view
# - ? for help overlay
# - Esc to go back

# Export formats
starbase export "query" --format=paths
starbase export "query" --format=json
starbase export "query" --format=markdown

# Collections & annotations
starbase collection create test-collection
starbase collection add test-collection "some repo"
starbase note "repo name" "This is a note"
starbase pin "repo name"

# Index management
starbase index rebuild
```

---

## Workflow Constraints

1. **Jujutsu commits** after each step passes tests
2. **Test-gated progress:** `go test ./...` must pass
3. **Run verification:** `make verify` before major commits

---

## Data Locations

| Path | Contents | Synced? |
|------|----------|---------|
| `~/.config/starbase/config.yaml` | Settings | ✅ (dotfiles) |
| `~/.config/starbase/manifest.yaml` | Tracked repos + annotations | ✅ (dotfiles) |
| `~/.local/share/starbase/starbase.db` | Cache, FTS index | ❌ (derived) |
| `~/.local/share/starbase/clones/` | Git clones | ❌ (derived) |

---

## Development Tips

### Symlink for development
```bash
ln -sf "$(pwd)/bin/starbase" ~/.local/bin/starbase
make build  # Updates binary, symlink follows
```

### Version includes build timestamp
```bash
starbase --version
# starbase version abc123-dirty (built 2026-01-31 10:50:13 UTC)
```
