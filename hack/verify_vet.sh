#!/usr/bin/env bash
set -euo pipefail

echo "==> Running go vet..."

go vet ./...

echo "✓ go vet passed"
