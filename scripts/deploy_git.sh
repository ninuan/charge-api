#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_HOST="${DEPLOY_HOST:-}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/charge-api}"
SERVICE_NAME="${SERVICE_NAME:-charge-api}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"
SSH_OPTS="${SSH_OPTS:-}"
SKIP_CHECK="${SKIP_CHECK:-0}"
DEPLOY_REMOTE="${DEPLOY_REMOTE:-origin}"
DEPLOY_BRANCH="${DEPLOY_BRANCH:-}"
DRY_RUN=0

usage() {
  cat <<'USAGE'
Usage:
  make deploy-git DEPLOY_HOST=root@server
  DEPLOY_BRANCH=main make deploy-git DEPLOY_HOST=root@server
  SKIP_CHECK=1 make deploy-git DEPLOY_HOST=root@server

Environment:
  DEPLOY_HOST      Required. SSH target, for example root@8.148.25.204.
  DEPLOY_PATH      Remote git project path. Default: /opt/charge-api.
  DEPLOY_REMOTE    Git remote name. Default: origin.
  DEPLOY_BRANCH    Git branch to push and pull. Default: current local branch.
  SERVICE_NAME     systemd service name. Default: charge-api.
  HEALTH_URL       Remote health check URL. Default: http://127.0.0.1:8080/healthz.
  SSH_OPTS         Extra ssh options, for example "-p 2222".
  SKIP_CHECK       Set to 1 to skip local make check.

Options:
  --dry-run        Print git/ssh actions without executing them.
  -h, --help       Show this help.
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
  echo "DEPLOY_HOST is required. Example: make deploy-git DEPLOY_HOST=root@8.148.25.204" >&2
  exit 1
fi

require_command git
require_command ssh

if [[ -z "$DEPLOY_BRANCH" ]]; then
  DEPLOY_BRANCH="$(git -C "$ROOT_DIR" branch --show-current)"
fi

if [[ -z "$DEPLOY_BRANCH" ]]; then
  echo "DEPLOY_BRANCH is required when the current commit is detached." >&2
  exit 1
fi

if [[ "$DRY_RUN" -ne 1 ]] && [[ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]]; then
  echo "Working tree has uncommitted changes. Commit or stash them before deploy-git." >&2
  exit 1
fi

if [[ "$SKIP_CHECK" != "1" ]]; then
  require_command make
  log "Run local verification"
  run_cmd make -C "$ROOT_DIR" check
else
  log "Skip local verification"
fi

log "Push local branch to GitHub"
run_cmd git -C "$ROOT_DIR" push "$DEPLOY_REMOTE" "$DEPLOY_BRANCH"

remote_path_quoted="$(printf '%q' "$DEPLOY_PATH")"
remote_quoted="$(printf '%q' "$DEPLOY_REMOTE")"
branch_quoted="$(printf '%q' "$DEPLOY_BRANCH")"

log "Pull, build and restart on remote server"
read -r -d '' remote_script <<REMOTE || true
set -Eeuo pipefail
cd $remote_path_quoted
git pull --ff-only $remote_quoted $branch_quoted
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

log "Git deploy finished"
