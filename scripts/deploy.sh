#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_HOST="${DEPLOY_HOST:-}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/charge-api}"
SERVICE_NAME="${SERVICE_NAME:-charge-api}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"
SSH_OPTS="${SSH_OPTS:-}"
SKIP_CHECK="${SKIP_CHECK:-0}"
NPM_REGISTRY="${NPM_REGISTRY:-}"
PNPM_FETCH_TIMEOUT="${PNPM_FETCH_TIMEOUT:-120000}"
PNPM_FETCH_RETRIES="${PNPM_FETCH_RETRIES:-5}"
PNPM_NETWORK_CONCURRENCY="${PNPM_NETWORK_CONCURRENCY:-8}"
DRY_RUN=0

usage() {
  cat <<'USAGE'
Usage:
  make deploy DEPLOY_HOST=root@server
  DEPLOY_HOST=root@server DEPLOY_PATH=/opt/charge-api make deploy
  SKIP_CHECK=1 make deploy DEPLOY_HOST=root@server

Environment:
  DEPLOY_HOST    Required. SSH target, for example root@8.148.25.204.
  DEPLOY_PATH    Remote project path. Default: /opt/charge-api.
  SERVICE_NAME   systemd service name. Default: charge-api.
  HEALTH_URL     Remote health check URL. Default: http://127.0.0.1:8080/healthz.
  SSH_OPTS       Extra ssh options, for example "-p 2222".
  SKIP_CHECK     Set to 1 to skip local make check.
  NPM_REGISTRY   Optional npm registry for this deployment, for example https://registry.npmmirror.com.
  PNPM_FETCH_TIMEOUT
                HTTP request timeout in milliseconds. Default: 120000.
  PNPM_FETCH_RETRIES
                Registry request retries. Default: 5.
  PNPM_NETWORK_CONCURRENCY
                Concurrent registry requests. Default: 8.

Options:
  --dry-run      Print rsync/ssh actions without executing them.
  -h, --help     Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

log() {
  printf '\n==> %s\n' "$1"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing command: $1" >&2
    exit 1
  fi
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '[dry-run]'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

run_ssh() {
  local command="$1"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '[dry-run] ssh'
    if [[ -n "$SSH_OPTS" ]]; then
      printf ' %s' "$SSH_OPTS"
    fi
    printf ' %q %q\n' "$DEPLOY_HOST" "$command"
    return 0
  fi

  # shellcheck disable=SC2086
  ssh $SSH_OPTS "$DEPLOY_HOST" "$command"
}

if [[ -z "$DEPLOY_HOST" ]]; then
  echo "DEPLOY_HOST is required. Example: make deploy DEPLOY_HOST=root@8.148.25.204" >&2
  exit 1
fi

require_command rsync
require_command ssh

if [[ "$SKIP_CHECK" != "1" ]]; then
  require_command make
  log "Run local verification"
  run_cmd make -C "$ROOT_DIR" check
else
  log "Skip local verification"
fi

remote_path_quoted="$(printf '%q' "$DEPLOY_PATH")"
npm_registry_quoted="$(printf '%q' "$NPM_REGISTRY")"
pnpm_fetch_timeout_quoted="$(printf '%q' "$PNPM_FETCH_TIMEOUT")"
pnpm_fetch_retries_quoted="$(printf '%q' "$PNPM_FETCH_RETRIES")"
pnpm_network_concurrency_quoted="$(printf '%q' "$PNPM_NETWORK_CONCURRENCY")"

log "Create remote directory"
run_ssh "mkdir -p $remote_path_quoted"

log "Sync project files"
rsync_args=(
  -az
  --delete
  --exclude ".git/"
  --exclude ".local/"
  --exclude ".env"
  --exclude ".env.*"
  --exclude "*.db"
  --exclude "*.db-*"
  --exclude "*.sqlite"
  --exclude "*.sqlite-*"
  --exclude "*.sqlite3"
  --exclude "*.sqlite3-*"
  --exclude "*.key"
  --exclude "*.log"
  --exclude "*.migrated"
  --exclude "charge_state.json"
  --exclude "initial-admin-password.txt"
  --exclude "20260601_202646/"
  --exclude "backend/charge-server"
  --exclude "backend/server"
  --exclude "frontend/dist/"
  --exclude "frontend/node_modules/"
  --exclude "node_modules/"
  --exclude "dist/"
  --exclude ".DS_Store"
)

if [[ "$DRY_RUN" -eq 1 ]]; then
  printf '[dry-run] rsync'
  printf ' %q' "${rsync_args[@]}"
  printf ' -e %q' "ssh $SSH_OPTS"
  printf ' %q %q\n' "$ROOT_DIR/" "$DEPLOY_HOST:$DEPLOY_PATH/"
else
  # shellcheck disable=SC2086
  rsync "${rsync_args[@]}" -e "ssh $SSH_OPTS" "$ROOT_DIR/" "$DEPLOY_HOST:$DEPLOY_PATH/"
fi

log "Build and restart on remote server"
read -r -d '' remote_script <<REMOTE || true
set -Eeuo pipefail
cd $remote_path_quoted
npm_registry=$npm_registry_quoted
pnpm_fetch_timeout=$pnpm_fetch_timeout_quoted
pnpm_fetch_retries=$pnpm_fetch_retries_quoted
pnpm_network_concurrency=$pnpm_network_concurrency_quoted
bash scripts/check_frontend_sources.sh
cd frontend
# 必须用专用旗标而不是 --config.<key>：后者把值按字符串塞进配置，
# network-concurrency 在 pnpm 内部有数字类型校验，冷 store 首装会直接报错。
pnpm_config_args=(
  "--fetch-timeout=\$pnpm_fetch_timeout"
  "--fetch-retries=\$pnpm_fetch_retries"
  "--network-concurrency=\$pnpm_network_concurrency"
)
if [[ -n "\$npm_registry" ]]; then
  pnpm_config_args+=("--registry=\$npm_registry")
fi
echo "Install frontend dependencies (timeout: \${pnpm_fetch_timeout}ms, retries: \$pnpm_fetch_retries, concurrency: \$pnpm_network_concurrency)"
pnpm "\${pnpm_config_args[@]}" install --frozen-lockfile --prefer-offline --reporter=append-only
pnpm run build:static
cd ../backend
go build -o charge-server.new ./cmd/server
mv charge-server.new charge-server
sudo systemctl restart $SERVICE_NAME
curl --silent --fail --max-time 15 $HEALTH_URL >/dev/null
echo "Remote service is healthy: $HEALTH_URL"
REMOTE

run_ssh "$remote_script"

log "Deploy finished"
