# Agent Instructions for starbase-cli

## Verification Harness

This project uses a strict verification-gated workflow. **Every code change must pass all verification gates before proceeding.**

### Running Verification

```bash
# Full verification (required after every change)
make verify
# or
./hack/verify_all.sh
```

### Verification Order (cheapest → costliest)

| Step | Command | Time | Purpose |
|------|---------|------|---------|
| 1. Format | `goimports -l .` | ~1s | Canonical Go formatting |
| 2. Tidy | `go mod tidy` (check) | ~1s | Module consistency |
| 3. Vet | `go vet ./...` | ~2s | Suspicious constructs |
| 4. Build | `go build ./...` | ~3s | Compile/type-check |
| 5. Lint | `golangci-lint run` | ~15s | Static analysis |
| 6. Test | `go test -race ./...` | ~10s | Unit tests |

**Stop on first failure.** Fix the issue before running subsequent steps.

### Fixing Common Issues

```bash
# Format issues
make fmt
# or: $(go env GOPATH)/bin/goimports -w .

# Module issues  
go mod tidy
```

## Development Workflow

### Before Every Code Change

1. Ensure previous verification passed: `make verify`
2. Understand the scope of change

### After Every Code Change

1. Run `make verify`
2. If any step fails:
   - **STOP immediately**
   - Fix the specific issue
   - Re-run verification from step 1
   - Do NOT proceed until all steps pass
3. Run `make install-user` to update the global `starbase` command
4. Only when all steps pass: commit with `jj commit -m "<message>"`

### Commit Protocol

Use [jujutsu](https://martinvonz.github.io/jj/) for version control:

```bash
# After verification passes
jj commit -m "phase-X.Y: description"

# Check status
jj status
jj log
```

### Step Contract (for complex changes)

Before implementing multi-file changes, declare:

```
Step: <description>

Scope:
  - path/to/file.go (modify FunctionName)
  - path/to/new_file.go (new file)

Tests:
  - TestFunctionName_Case1
  - TestFunctionName_Case2

Verification: make verify
```

## Project Structure

```
starbase-cli/
├── cmd/starbase/     # CLI commands (Cobra)
├── internal/
│   ├── config/       # Configuration & manifest
│   ├── database/     # SQLite + FTS5
│   ├── forge/        # Multi-forge interface
│   │   └── github/   # GitHub implementation
│   ├── git/          # Git operations + worker pool
│   ├── index/        # Document extraction
│   ├── search/       # BM25 search
│   ├── sync/         # Sync orchestration
│   └── tui/          # Bubbletea TUI
├── hack/             # Verification scripts
├── docs/             # Documentation
└── bin/              # Build output
```

## Key Commands

```bash
# Build
make build

# Test (verbose)
make test

# Lint only
make lint

# Format only
make fmt

# Full verification
make verify

# Install to ~/.local/bin
make install-user
```

## Code Conventions

- **Formatting**: `goimports` is canonical authority
- **Tests**: Table-driven tests preferred
- **Errors**: Wrap with context using `fmt.Errorf("context: %w", err)`
- **Comments**: Avoid unless explaining non-obvious behavior
- **Dependencies**: Check existing before adding new

## Critical Rules

1. **Never skip verification gates** — no "TODO: fix later"
2. **Never commit failing code** — all gates must pass
3. **Never defer lint issues** — fix immediately
4. **Run full verification after every change** — not just the file you edited
5. **Treat first failure as only task** — stop everything else until fixed

## Tools Required

Ensure these are installed (run once):

```bash
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Add `$(go env GOPATH)/bin` to your PATH if not already.
