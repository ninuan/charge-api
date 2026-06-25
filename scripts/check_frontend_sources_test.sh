#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECK_SCRIPT="$ROOT_DIR/scripts/check_frontend_sources.sh"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$TMP_DIR/frontend/src/stores"
cat >"$TMP_DIR/frontend/src/stores/auth.js" <<'JS'
export const stale = true;
JS

set +e
output="$(bash "$CHECK_SCRIPT" "$TMP_DIR" 2>&1)"
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "check_frontend_sources should fail when JavaScript files exist in frontend/src"
  exit 1
fi

if [[ "$output" != *"frontend/src/stores/auth.js"* ]]; then
  echo "check_frontend_sources should print the offending file path"
  exit 1
fi

rm "$TMP_DIR/frontend/src/stores/auth.js"
mkdir -p "$TMP_DIR/frontend/src/components"
cat >"$TMP_DIR/frontend/src/components/App.vue" <<'VUE'
<template><main /></template>
VUE
cat >"$TMP_DIR/frontend/src/stores/auth.ts" <<'TS'
export const fresh = true;
TS

bash "$CHECK_SCRIPT" "$TMP_DIR" >/dev/null
