#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

backend_version="$(sed -n 's/^const Current = "\([^"]*\)"$/\1/p' "$ROOT_DIR/backend/internal/version/version.go")"
if [[ -z "$backend_version" ]]; then
  echo "无法读取后端发布版本" >&2
  exit 1
fi

frontend_version="$(cd "$ROOT_DIR/frontend" && node -p 'require("./package.json").version')"
openapi_rel="docs/openapi/charge-console-v$backend_version.yaml"
openapi_file="$ROOT_DIR/$openapi_rel"
if [[ ! -f "$openapi_file" ]]; then
  echo "缺少 $backend_version 对应的 OpenAPI 文档：$openapi_file" >&2
  exit 1
fi
openapi_version="$(sed -n 's/^  version: //p' "$openapi_file" | head -1)"

for source in "frontend/package.json:$frontend_version" "OpenAPI:$openapi_version"; do
  name="${source%%:*}"
  value="${source#*:}"
  if [[ "$value" != "$backend_version" ]]; then
    echo "发布版本不一致：后端为 $backend_version，$name 为 $value" >&2
    exit 1
  fi
done

for script_key in "check:api" "generate:api"; do
  script_cmd="$(cd "$ROOT_DIR/frontend" && node -e 'const pkg = require("./package.json"); console.log((pkg.scripts && pkg.scripts[process.argv[1]]) || "");' "$script_key")"
  if [[ "$script_cmd" != *"../$openapi_rel"* ]]; then
    echo "frontend/package.json 的 scripts.${script_key} 未指向 ../$openapi_rel：$script_cmd" >&2
    exit 1
  fi
done

if ! grep -q "^## $backend_version -" "$ROOT_DIR/CHANGELOG.md"; then
  echo "CHANGELOG.md 缺少 $backend_version 发布说明" >&2
  exit 1
fi

echo "release version 检查通过：$backend_version"
