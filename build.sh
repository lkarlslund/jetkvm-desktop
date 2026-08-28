#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

mkdir -p dist
go build -trimpath -ldflags="-s -w" -o dist/jetkvm-desktop ./cmd/jetkvm-desktop

echo "Built: dist/jetkvm-desktop"
