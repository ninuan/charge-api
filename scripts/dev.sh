#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATABASE_FILE="${LOCAL_DATABASE_FILE:-$ROOT_DIR/.local/charge_state.db}"
LEGACY_STATE_FILE="${LOCAL_STATE_FILE:-$ROOT_DIR/.local/charge_state.json}"
COOKIE_KEY_FILE="${LOCAL_COOKIE_KEY_FILE:-$ROOT_DIR/.local/cookie.key}"
ADMIN_PASSWORD="${LOCAL_ADMIN_PASSWORD:-localadmin123}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
BACKEND_PID=""
FRONTEND_PID=""

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少命令: $1。请先运行 make setup 或安装对应运行环境。"
    exit 1
  fi
}

check_port() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1 && lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "端口 $port 已被占用，请先停止对应服务。"
    exit 1
  fi
}

wait_for_backend() {
  local attempts=120
  for ((i = 1; i <= attempts; i++)); do
    if ! kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
      echo "后端启动失败，请查看上方 Go 错误信息。"
      wait "$BACKEND_PID" || true
      return 1
    fi
    if curl --silent --fail "http://127.0.0.1:$BACKEND_PORT/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "等待后端启动超时（120 秒）。"
  return 1
}

cleanup() {
  trap - EXIT
  if [[ -n "$FRONTEND_PID" ]] && kill -0 "$FRONTEND_PID" >/dev/null 2>&1; then
    kill "$FRONTEND_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$BACKEND_PID" ]] && kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    kill "$BACKEND_PID" >/dev/null 2>&1 || true
  fi
  wait "$FRONTEND_PID" "$BACKEND_PID" 2>/dev/null || true
  echo
  echo "本地前后端已停止。"
}

require_command go
require_command npm
require_command curl
check_port "$BACKEND_PORT"
check_port "$FRONTEND_PORT"

mkdir -p "$(dirname "$DATABASE_FILE")"
mkdir -p "$(dirname "$COOKIE_KEY_FILE")"

if [[ -n "${CHARGE_COOKIE_KEY:-}" ]]; then
  COOKIE_KEY="$CHARGE_COOKIE_KEY"
elif [[ -f "$COOKIE_KEY_FILE" ]]; then
  COOKIE_KEY="$(cat "$COOKIE_KEY_FILE")"
else
  require_command openssl
  umask 077
  COOKIE_KEY="$(openssl rand -base64 32)"
  printf '%s\n' "$COOKIE_KEY" >"$COOKIE_KEY_FILE"
fi

if [[ ! -d "$ROOT_DIR/frontend/node_modules" ]]; then
  echo "前端依赖尚未安装，正在执行 npm ci..."
  (cd "$ROOT_DIR/frontend" && npm ci)
fi

trap cleanup EXIT
trap 'exit 0' INT TERM

echo "启动本地开发环境..."
echo "前端地址: http://127.0.0.1:$FRONTEND_PORT"
echo "管理员账号: admin"
if [[ -f "$DATABASE_FILE" ]] || [[ -f "$LEGACY_STATE_FILE" ]]; then
  echo "管理员密码: 使用现有数据库中的密码"
else
  echo "管理员密码: $ADMIN_PASSWORD"
fi
echo "本地数据库: $DATABASE_FILE"
echo "Cookie 密钥: $COOKIE_KEY_FILE"
echo "按 Ctrl+C 同时停止前后端。"
echo

(
  cd "$ROOT_DIR/backend"
  TURNSTILE_REQUIRED=true \
  TURNSTILE_SITE_KEY=1x00000000000000000000AA \
  TURNSTILE_SECRET_KEY=1x0000000000000000000000000000000AA \
  CORS_ALLOWED_ORIGINS="http://127.0.0.1:$FRONTEND_PORT" \
  CHARGE_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
  CHARGE_COOKIE_KEY="$COOKIE_KEY" \
  go run ./cmd/server \
    -listen "127.0.0.1:$BACKEND_PORT" \
    -database "$DATABASE_FILE" \
    -state "$LEGACY_STATE_FILE"
) &
BACKEND_PID=$!

echo "等待 Go 后端完成编译并通过健康检查..."
wait_for_backend
echo "Go 后端已就绪。"

(
  cd "$ROOT_DIR/frontend"
  VITE_API_TARGET="http://127.0.0.1:$BACKEND_PORT" \
  npm run dev -- --host 127.0.0.1 --port "$FRONTEND_PORT"
) &
FRONTEND_PID=$!

while kill -0 "$BACKEND_PID" >/dev/null 2>&1 && kill -0 "$FRONTEND_PID" >/dev/null 2>&1; do
  sleep 1
done

echo "检测到一个服务已退出，正在停止另一个服务。"
exit 1
