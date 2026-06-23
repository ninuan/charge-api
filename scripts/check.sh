#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_CACHE="${GOCACHE:-${TMPDIR:-/tmp}/charge-go-cache}"
BUILD_OUTPUT="${TMPDIR:-/tmp}/charge-check-server"

if [[ ! -d "$ROOT_DIR/frontend/node_modules" ]]; then
  echo "前端依赖尚未安装，请先运行 make setup。"
  exit 1
fi

echo "1/4 Go 测试"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go test ./...
)

echo "2/4 Go 构建"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go build -o "$BUILD_OUTPUT" ./cmd/server
)

echo "3/4 前端测试"
(
  cd "$ROOT_DIR/frontend"
  npm test
)

echo "4/4 前端类型检查与生产构建"
(
  cd "$ROOT_DIR/frontend"
  npm run build
)

echo
echo "全部检查通过，可以提交或部署。"
