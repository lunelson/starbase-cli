#!/usr/bin/env bash
set -euo pipefail

echo "==> Running unit tests..."

go test -v -race -cover ./...

echo "✓ Unit tests passed"
