#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNTIME_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-e2e.XXXXXX")"
BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
  trap - EXIT
  if [[ -n "$FRONTEND_PID" ]] && kill -0 "$FRONTEND_PID" >/dev/null 2>&1; then
    kill "$FRONTEND_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$BACKEND_PID" ]] && kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    kill "$BACKEND_PID" >/dev/null 2>&1 || true
  fi
  wait "$FRONTEND_PID" "$BACKEND_PID" 2>/dev/null || true
  rm -rf "$RUNTIME_DIR"
}

trap cleanup EXIT
trap 'exit 0' INT TERM

(
  cd "$ROOT_DIR/backend"
  TURNSTILE_REQUIRED=false \
  CORS_ALLOWED_ORIGINS="http://127.0.0.1:3000" \
  CHARGE_ADMIN_PASSWORD=localadmin123 \
  CHARGE_COOKIE_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA= \
  CHARGE_LOCAL_DEV=1 \
  go run ./cmd/server \
    -listen 127.0.0.1:8080 \
    -database "$RUNTIME_DIR/state.db" \
    -state "$RUNTIME_DIR/state.json"
) &
BACKEND_PID=$!

for _ in {1..120}; do
  if curl --silent --fail http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    wait "$BACKEND_PID"
  fi
  sleep 1
done

if ! curl --silent --fail http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
  echo "E2E backend did not become ready." >&2
  exit 1
fi

(
  cd "$ROOT_DIR/frontend"
  NEXT_PUBLIC_API_TARGET=http://127.0.0.1:8080 \
  pnpm exec next dev --hostname 127.0.0.1 --port 3000
) &
FRONTEND_PID=$!

wait "$FRONTEND_PID"
