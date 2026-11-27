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
  DEPLOY_HOST    Required. SSH target, for example root@your-server.
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
  echo "DEPLOY_HOST is required. Example: make deploy DEPLOY_HOST=root@your-server" >&2
  exit 1
fi

if [[ ! "$SERVICE_NAME" =~ ^[A-Za-z0-9_.@-]+$ ]]; then
  echo "SERVICE_NAME contains unsupported characters" >&2
  exit 2
fi
if [[ "$HEALTH_URL" != http://* && "$HEALTH_URL" != https://* ]] || [[ "$HEALTH_URL" =~ [[:space:][:cntrl:]] ]]; then
  echo "HEALTH_URL must use http:// or https://" >&2
  exit 2
fi
if [[ ! "$DEPLOY_PATH" =~ ^/[A-Za-z0-9._/@-]+$ ]] \
  || [[ "$DEPLOY_PATH" == *"//"* ]] \
  || [[ "$DEPLOY_PATH" =~ (^|/)(\.|\.\.)(/|$) ]] \
  || [[ "${DEPLOY_PATH%/}" == "" ]]; then
  echo "Invalid DEPLOY_PATH: $DEPLOY_PATH" >&2
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
service_name_quoted="$(printf '%q' "$SERVICE_NAME")"
health_url_quoted="$(printf '%q' "$HEALTH_URL")"

log "Create remote directory"
run_ssh "mkdir -p $remote_path_quoted"

log "Sync project files"

rsync_args=(
  -az
  --no-owner
  --no-group
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
  --exclude "backend/charge-server"
  --exclude "backend/server"
  --exclude "frontend/.next/"
  --exclude "frontend/out/"
  --exclude "frontend/.env*.local"
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
for tool in go node pnpm rsync curl; do
  if ! command -v "\$tool" >/dev/null 2>&1; then
    echo "服务器缺少 \$tool，请先按 README「部署同步 - 服务器构建环境」安装后重试" >&2
    exit 1
  fi
done
if ! node -e 'const [major, minor] = process.versions.node.split(".").map(Number); process.exit(major > 20 || (major === 20 && minor >= 9) ? 0 : 1)'; then
  echo "服务器 Node 版本过低（\$(node --version)），Next 16 需要 >= 20.9，建议安装 Node 22" >&2
  exit 1
fi
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
pnpm exec next build
cd ../backend
go build -o charge-server.new ./cmd/server
rm -f charge-server.prev
if [[ -f charge-server ]]; then
  cp -p charge-server charge-server.prev
fi
rm -rf ../frontend/dist.new ../frontend/dist.prev ../frontend/dist.failed
rsync -a --delete ../frontend/out/ ../frontend/dist.new/
service_name=$service_name_quoted
health_url=$health_url_quoted
rollback_release() {
  if [[ -d ../frontend/dist.prev ]]; then
    echo "Rolling back frontend/dist to previous static bundle..." >&2
    if [[ -d ../frontend/dist ]]; then
      mv ../frontend/dist ../frontend/dist.failed || true
    fi
    mv ../frontend/dist.prev ../frontend/dist || true
    rm -rf ../frontend/dist.failed || true
  fi
  if [[ -f charge-server.prev ]]; then
    echo "Rolling back charge-server to previous binary..." >&2
    mv -f charge-server.prev charge-server || true
    sudo systemctl restart "\$service_name" || true
  fi
}
if [[ -d ../frontend/dist ]]; then
  mv ../frontend/dist ../frontend/dist.prev
fi
if ! mv ../frontend/dist.new ../frontend/dist || ! mv charge-server.new charge-server; then
  rollback_release
  exit 1
fi
restart_ok=1
sudo systemctl restart "\$service_name" || restart_ok=0
health_ready=0
if [[ "\$restart_ok" -eq 1 ]]; then
  for _ in {1..15}; do
    if sudo systemctl is-active --quiet "\$service_name" && curl --silent --fail --max-time 2 "\$health_url" | grep -q '"status":"ok"'; then
      health_ready=1
      break
    fi
    sleep 1
  done
fi
if [[ "\$health_ready" -ne 1 ]]; then
  echo "Charge health check failed after restart. Recent service diagnostics:" >&2
  sudo systemctl --no-pager --full status "\$service_name" || true
  sudo journalctl -u "\$service_name" -n 80 --no-pager || true
  rollback_release
  exit 1
fi
rm -rf charge-server.prev ../frontend/dist.prev ../frontend/dist.failed
echo "Remote service is healthy: \$health_url"
REMOTE

run_ssh "$remote_script"

log "Deploy finished"
