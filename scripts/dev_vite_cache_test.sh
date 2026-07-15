#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_SCRIPT="$ROOT_DIR/scripts/dev.sh"

if ! grep -Eq 'pnpm dev -- --hostname 127\.0\.0\.1 --port' "$DEV_SCRIPT"; then
  echo "dev script should start the Next development server on the configured local port"
  exit 1
fi
