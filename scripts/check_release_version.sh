#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

backend_version="$(sed -n 's/^const Current = "\([^"]*\)"$/\1/p' "$ROOT_DIR/backend/internal/version/version.go")"
frontend_version="$(cd "$ROOT_DIR/frontend" && node -p 'require("./package.json").version')"
openapi_version="$(sed -n 's/^  version: //p' "$ROOT_DIR/docs/openapi/charge-console-v1.5.0.yaml" | head -1)"

if [[ -z "$backend_version" ]]; then
  echo "无法读取后端发布版本" >&2
  exit 1
fi

for source in "frontend/package.json:$frontend_version" "OpenAPI:$openapi_version"; do
  name="${source%%:*}"
  value="${source#*:}"
  if [[ "$value" != "$backend_version" ]]; then
    echo "发布版本不一致：后端为 $backend_version，$name 为 $value" >&2
    exit 1
  fi
done

if ! grep -q "^## $backend_version -" "$ROOT_DIR/CHANGELOG.md"; then
  echo "CHANGELOG.md 缺少 $backend_version 发布说明" >&2
  exit 1
fi

echo "release version 检查通过：$backend_version"
