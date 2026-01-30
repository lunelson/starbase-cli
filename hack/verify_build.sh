#!/usr/bin/env bash
set -euo pipefail

echo "==> Compiling all packages..."

go build ./...

echo "✓ Build passed"
