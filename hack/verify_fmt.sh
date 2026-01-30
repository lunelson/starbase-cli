#!/usr/bin/env bash
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

echo "==> Checking Go formatting..."

DIFF=$(goimports -l .)
if [ -n "$DIFF" ]; then
    echo "ERROR: The following files are not formatted correctly:"
    echo "$DIFF"
    echo ""
    echo "Run: goimports -w ."
    exit 1
fi

echo "✓ All files properly formatted"
