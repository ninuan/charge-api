#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
SRC_DIR="$ROOT_DIR/frontend/src"

if [[ ! -d "$SRC_DIR" ]]; then
  echo "Frontend source directory not found: $SRC_DIR" >&2
  exit 1
fi

js_sources="$(
  find "$SRC_DIR" -type f \
    \( -name '*.js' -o -name '*.jsx' -o -name '*.mjs' -o -name '*.cjs' \) \
    -print | sort
)"

if [[ -n "$js_sources" ]]; then
  echo "检测到 frontend/src 中存在 JavaScript 源文件。" >&2
  echo "这个项目的前端源码应使用 TypeScript/Vue；无后缀 import 可能优先命中旧 JS，导致 TS/Vue 修改没有进入构建。" >&2
  echo "请删除或改名这些文件：" >&2
  printf '%s\n' "$js_sources" | sed 's/^/  /' >&2
  exit 1
fi
