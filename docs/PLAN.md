# starbase-cli: Development Plan

> Last updated: 2026-01-29

## Current Status

**Phases Complete:**
- ✅ Phase 1: Foundation (config, manifest, SQLite+FTS5, init)
- ✅ Phase 2: GitHub integration (forge interface, API client, git ops, sync)
- ✅ Phase 3: Search (FTS5 BM25, search/list commands, document extraction)
- ✅ Phase 4: Export (export command, collections, notes, pins)
- ✅ Phase 5.1: TUI basics (Bubbletea browse with list/search)

**All tests passing.** Binary builds successfully.

---

## Immediate Next Steps

### Step 0: Installation & First Sync (User Testing)

Before building new features, validate everything works end-to-end:

```bash
# 1. Build and install globally
make install
# OR add to PATH:
export PATH="$PATH:$(pwd)/bin"

# 2. Initialize starbase
starbase init

# 3. Verify GitHub auth
# Option A: Use gh CLI (recommended)
gh auth status

# Option B: Set token directly
export STARBASE_GITHUB_TOKEN="ghp_..."

# 4. Dry-run sync to see what would happen
starbase sync --since=6mo --dry-run

# 5. First real sync (sequential, may be slow)
starbase sync --since=6mo

# 6. Test search
starbase search "golang cli"
starbase list --format=json | head -20

# 7. Test TUI
starbase browse
```

### Step 1: Parallel Git Operations

**Problem:** Cloning 100+ repos sequentially is slow (network-bound).

**Solution:** Worker pool for git clone/pull operations.

```
internal/git/pool.go  - Worker pool implementation
```

**Design:**
- Configurable concurrency (default: 4, max: 10)
- Progress callback for UI updates
- Error collection (don't fail fast)
- Context cancellation support

**Config addition:**
```yaml
sync:
  concurrency: 4  # parallel git operations
```

**Commit:** `jj commit -m "phase-5.5: parallel git operations"`

---

## Remaining Phases

### Phase 5.2-5.5: Enhanced TUI

| Step | Feature | Description |
|------|---------|-------------|
| 5.2 | Multi-select | Space to toggle, select all/none |
| 5.3 | Detail view | Enter on single repo shows full metadata + README preview |
| 5.4 | Actions | Open in $EDITOR, browser, copy path to clipboard |
| 5.5 | Help overlay | `?` shows keybindings |

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
- `internal/git/pool_test.go` - Worker pool logic
- `internal/tui/*_test.go` - TUI model updates

### Integration Tests
- `internal/sync/sync_test.go` - Full sync with mock forge
- `cmd/starbase/*_test.go` - CLI command tests

### Manual Testing Checklist

After each phase, verify:

```bash
# Core functionality
starbase init                    # Creates ~/.config/starbase/, ~/.local/share/starbase/
starbase sync --since=30d        # Fetches stars, clones repos
starbase search "test query"     # Returns ranked results
starbase list                    # Shows all repos
starbase browse                  # TUI launches

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
3. **Run verification:** `make lint` before major commits

---

## Data Locations

| Path | Contents | Synced? |
|------|----------|---------|
| `~/.config/starbase/config.yaml` | Settings | ✅ (dotfiles) |
| `~/.config/starbase/manifest.yaml` | Tracked repos + annotations | ✅ (dotfiles) |
| `~/.local/share/starbase/starbase.db` | Cache, FTS index | ❌ (derived) |
| `~/.local/share/starbase/clones/` | Git clones | ❌ (derived) |
