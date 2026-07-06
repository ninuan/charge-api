#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_SCRIPT="$ROOT_DIR/scripts/dev.sh"

if ! grep -Eq 'npm run dev -- .*--force' "$DEV_SCRIPT"; then
  echo "dev script should start Vite with --force to avoid stale optimizeDeps chunks"
  exit 1
fi
