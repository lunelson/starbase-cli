#!/usr/bin/env bash
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

echo "==> Running golangci-lint..."

golangci-lint run --timeout=5m

echo "✓ golangci-lint passed"
