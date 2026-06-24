#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_HOST="${DEPLOY_HOST:-}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/charge-api}"
SERVICE_NAME="${SERVICE_NAME:-charge-api}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"
SSH_OPTS="${SSH_OPTS:-}"
SKIP_CHECK="${SKIP_CHECK:-0}"
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
cd frontend
npm ci
npm run build
cd ../backend
go build -o charge-server.new ./cmd/server
mv charge-server.new charge-server
sudo systemctl restart $SERVICE_NAME
curl --silent --fail --max-time 15 $HEALTH_URL >/dev/null
echo "Remote service is healthy: $HEALTH_URL"
REMOTE

run_ssh "$remote_script"

log "Deploy finished"
