#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-announcements.XXXXXX")"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
cd "$ROOT_DIR/backend"
go build -o "$TEST_DIR/server" ./cmd/server
# Only isolated local fixtures; never use the developer's database or integrations.
env -u YYB_BASE_URL -u YYB_API_SECRET -u MOCELE_BASE_URL -u WXPUSHER_APP_TOKEN \
 CORS_ALLOWED_ORIGINS=http://127.0.0.1:18081 PUBLIC_BASE_URL=http://127.0.0.1:18081 CHARGE_LOCAL_DEV=0 \
 CHARGE_ADMIN_PASSWORD=local-announcement-test-password \
 CHARGE_COOKIE_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA= \
 "$TEST_DIR/server" -listen 127.0.0.1:18081 -database "$TEST_DIR/state.db" -state "$TEST_DIR/absent.json" &
SERVER_PID=$!
wait "$SERVER_PID"
