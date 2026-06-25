#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_CACHE="${GOCACHE:-${TMPDIR:-/tmp}/charge-go-cache}"
BUILD_OUTPUT="${TMPDIR:-/tmp}/charge-check-server"

if [[ ! -d "$ROOT_DIR/frontend/node_modules" ]]; then
  echo "前端依赖尚未安装，请先运行 make setup。"
  exit 1
fi

echo "1/6 部署脚本检查"
bash "$ROOT_DIR/scripts/deploy_test.sh"
bash "$ROOT_DIR/scripts/deploy_git_test.sh"
bash "$ROOT_DIR/scripts/check_frontend_sources_test.sh"

echo "2/6 前端源码检查"
bash "$ROOT_DIR/scripts/check_frontend_sources.sh"

echo "3/6 Go 测试"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go test ./...
)

echo "4/6 Go 构建"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go build -o "$BUILD_OUTPUT" ./cmd/server
)

echo "5/6 前端测试"
(
  cd "$ROOT_DIR/frontend"
  npm test
)

echo "6/6 前端类型检查与生产构建"
(
  cd "$ROOT_DIR/frontend"
  npm run build
)

echo
echo "全部检查通过，可以提交或部署。"
