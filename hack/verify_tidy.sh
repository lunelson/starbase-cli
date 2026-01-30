#!/usr/bin/env bash
set -euo pipefail

echo "==> Checking go.mod is tidy..."

cp go.mod go.mod.bak
cp go.sum go.sum.bak

go mod tidy

if ! diff -q go.mod go.mod.bak > /dev/null 2>&1 || ! diff -q go.sum go.sum.bak > /dev/null 2>&1; then
    echo "ERROR: go.mod or go.sum is not tidy"
    echo "Run: go mod tidy"
    mv go.mod.bak go.mod
    mv go.sum.bak go.sum
    exit 1
fi

rm go.mod.bak go.sum.bak
echo "✓ go.mod is tidy"
