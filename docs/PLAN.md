# starbase-cli: Development Plan

> Last updated: 2026-01-31

---

## What's Done

### Core Features (Complete)
- **Config & Storage**: XDG-compliant paths, YAML config, SQLite+FTS5 database
- **GitHub Integration**: Forge interface, API client, star/unstar, pagination
- **Sync Engine**: Parallel git operations, progress feedback, recency window
- **Search**: BM25 full-text search via FTS5, document extraction
- **CLI Commands**: init, sync, search, list, add, rm, prune, export
- **TUI (browse)**: List/search views, detail view with Glamour, multi-select, actions (e/w/y), help overlay
- **Collections & Annotations**: notes, pins, collections in manifest
- **Tombstones**: Removed repos stored in manifest, skipped during sync

### Recent Session (Repository Identity Redesign)
- ✅ URL parser accepting multiple formats (HTTPS, SSH, web UI URLs, short form)
- ✅ `add` command accepts any URL format
- ✅ `rm` command with `--delete` and `--unstar` flags (default: keep files + star)
- ✅ Tombstones in manifest (cross-machine sync)
- ✅ Sync respects tombstones

---

## What's Next

### Immediate (Current Subphase)

| Task | Effort | Notes |
|------|--------|-------|
| `rm` interactive mode | Small | Multiselect when no repo arg provided |
| DB schema migration to `host:owner/repo` IDs | Medium | Currently deferred; tombstones use new format, DB uses ForgeID |

### Small Tasks (Ready Now)

| Task | Effort | Notes |
|------|--------|-------|
| `add` interactive mode | Small | Search/select from untracked stars |
| TUI: batch actions on selection | Small | e/w/y on multiple selected repos |
| TUI: filtering by language/topic | Small | Add filter UI to list view |
| Config validation on load | Small | Warn on unknown keys, validate paths |
| `sync --prune` flag | Small | Combine sync + prune in one command |

### Medium Tasks (Self-Contained)

| Task | Effort | Notes |
|------|--------|-------|
| GitLab forge implementation | Medium | New API client, reuse forge interface |
| Workspace export | Medium | Copy selected repos + generate CONTEXT.md |
| Auth commands (device flow) | Medium | `starbase auth login`, keyring storage |
| Clone path customization | Medium | User-defined path templates |
| Stats/dashboard command | Medium | Show sync stats, storage usage, language breakdown |

### Large Tasks (Infrastructure Required)

| Task | Effort | Dependencies |
|------|--------|--------------|
| **Vector embeddings** | Large | Ollama or OpenAI API |
| Hybrid search (BM25 + semantic) | Large | Embeddings infrastructure |
| MCP server mode | Large | Protocol implementation, daemon |

---

## Architecture Reference

### Data Flow
```
GitHub API → Sync → Database → Search Index
                 ↘ Git Clone → Document Extraction ↗
```

### Data Locations

| Path | Contents | Synced? |
|------|----------|---------|
| `~/.config/starbase/config.yaml` | Settings | ✅ (dotfiles) |
| `~/.config/starbase/manifest.yaml` | Annotations, tombstones | ✅ (dotfiles) |
| `~/.local/share/starbase/starbase.db` | Cache, FTS index | ❌ (derived) |
| `~/.local/share/starbase/clones/` | Git clones | ❌ (derived) |

### TUI Structure
```
internal/tui/
├── model.go   # Main model, viewState enum, Update/View
├── keys.go    # Keybindings with help text
└── styles.go  # Lipgloss styles
```

**Views:** listView → searchView → detailView → helpView

**Key Bindings:**
| Key | Action |
|-----|--------|
| `j/k` | Navigate |
| `space` | Toggle select |
| `a/n` | Select all/none |
| `/` | Search |
| `Enter` | Detail view |
| `e/w/y` | Editor/browser/clipboard |
| `?` | Help |
| `q` | Quit |

---

## Design Decisions

### Repository ID Format
- **Tombstones/manifest**: `host:owner/repo` (e.g., `github.com:charmbracelet/bubbletea`)
- **Database**: Still uses GitHub's `ForgeID` for now (migration deferred)
- **Rationale**: Human-readable, multi-forge compatible

### URL Parsing
Accepts any of:
- `owner/repo` → assumes github.com
- `https://github.com/owner/repo`
- `https://github.com/owner/repo/tree/main/...`
- `git@github.com:owner/repo.git`

### rm Command Semantics
| Flags | Effect |
|-------|--------|
| (none) | Remove from DB, keep files, keep star, add tombstone |
| `--delete` | Also delete local clone |
| `--unstar` | Also unstar on GitHub |
| `-d --unstar` | Full removal |

---

## Development Workflow

```bash
# After every change
make verify         # Format, vet, build, lint, test
make install-user   # Update global binary
jj commit -m "..."  # Commit with jujutsu
```

### Manual Testing
```bash
starbase init
starbase sync --since=30d
starbase search "query"
starbase list
starbase browse
starbase add owner/repo
starbase rm owner/repo --delete
```
