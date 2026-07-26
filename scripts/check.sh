#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_CACHE="${GOCACHE:-${TMPDIR:-/tmp}/charge-go-cache}"
BUILD_OUTPUT="${TMPDIR:-/tmp}/charge-check-server"

if [[ ! -d "$ROOT_DIR/frontend/node_modules" ]]; then
  echo "前端依赖尚未安装，请先运行 make setup。"
  exit 1
fi

echo "1/7 部署脚本检查"
bash "$ROOT_DIR/scripts/deploy_test.sh"
bash "$ROOT_DIR/scripts/deploy_git_test.sh"
bash "$ROOT_DIR/scripts/dev_env_test.sh"
bash "$ROOT_DIR/scripts/dev_vite_cache_test.sh"
bash "$ROOT_DIR/scripts/check_frontend_sources_test.sh"
bash "$ROOT_DIR/scripts/ops_hardening_test.sh"
bash "$ROOT_DIR/scripts/security_check_test.sh"

echo "2/7 前端源码检查"
bash "$ROOT_DIR/scripts/check_frontend_sources.sh"

echo "3/7 前端 lint"
(
  cd "$ROOT_DIR/frontend"
  pnpm lint
)

echo "4/7 Go 测试"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go test ./...
)

echo "5/7 Go 构建"
(
  cd "$ROOT_DIR/backend"
  GOCACHE="$GO_CACHE" go build -o "$BUILD_OUTPUT" ./cmd/server
)

echo "6/7 前端测试"
(
  cd "$ROOT_DIR/frontend"
  pnpm test
)

echo "7/7 前端类型检查与生产构建"
(
  cd "$ROOT_DIR/frontend"
  pnpm run build:static
)

echo
echo "全部检查通过，可以提交或部署。"
