#!/usr/bin/env bash
# Run all project tests: Go unit tests + UI/i18n tests.
set -euo pipefail
cd "$(dirname "$0")/.."

# Pure-Go build: no gcc/cgo needed, works in a bare nix-shell.
export CGO_ENABLED=0

echo "===== Go tests ====="
go test ./...

echo
echo "===== UI / i18n tests ====="
node tests/ui_i18n.test.mjs

echo
echo "ALL TESTS PASSED"
