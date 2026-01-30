#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

STEPS=(
    "hack/verify_fmt.sh"
    "hack/verify_tidy.sh"
    "hack/verify_vet.sh"
    "hack/verify_build.sh"
    "hack/verify_lint.sh"
    "hack/verify_test.sh"
)

for step in "${STEPS[@]}"; do
    echo ""
    echo "========================================"
    echo "Running: $step"
    echo "========================================"
    if ! ./"$step"; then
        echo ""
        echo "FAILED: $step"
        echo "Fix this before proceeding."
        exit 1
    fi
done

echo ""
echo "========================================"
echo "✓ All verification steps passed"
echo "========================================"
