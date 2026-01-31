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
- ✅ Phase 5.5: Help overlay (? shows keybindings)

**All tests passing.** Binary builds successfully.

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

## Immediate Next Steps

### Phase 5.4: Actions

Implement action keys that work from both list and detail views:

| Key | Action | Implementation |
|-----|--------|----------------|
| `e` | Open in $EDITOR | `exec.Command(os.Getenv("EDITOR"), localPath)` |
| `w` | Open in browser | `exec.Command("open", webURL)` (macOS) |
| `y` | Copy path | Clipboard via `golang.design/x/clipboard` |

**Files to modify:**
- `internal/tui/model.go` — Add action handlers
- Add clipboard dependency

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
| 7.3 | `add` / `remove` commands |

### Phase 8: Advanced Features

| Step | Feature |
|------|---------|
| 8.1 | Prune command |
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
