#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT_DIR/README.md"
DEPLOYMENT_GUIDE="$ROOT_DIR/docs/deployment.md"
CHARGE_SERVICE="$ROOT_DIR/deploy/systemd/charge.service"
BACKUP_SERVICE="$ROOT_DIR/deploy/systemd/charge-backup.service"

require_file() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    echo "missing required file: $file"
    exit 1
  fi
}

require_contains() {
  local file="$1"
  local needle="$2"
  if ! grep -Fq -- "$needle" "$file"; then
    echo "expected '$file' to contain: $needle"
    exit 1
  fi
}

require_file "$README"
require_file "$DEPLOYMENT_GUIDE"
require_file "$CHARGE_SERVICE"
require_file "$BACKUP_SERVICE"

if [[ -e "$ROOT_DIR/deploy/systemd/yyb-go.service" ]]; then
  echo "retired yyb-go.service should not exist"
  exit 1
fi

require_contains "$README" "[生产部署与运维指南](docs/deployment.md)"
require_contains "$DEPLOYMENT_GUIDE" "## 安全加固"
require_contains "$DEPLOYMENT_GUIDE" "/opt/charge-api/backend/charge-server"
require_contains "$DEPLOYMENT_GUIDE" "-listen 127.0.0.1:8080"
require_contains "$DEPLOYMENT_GUIDE" "stat -c '%a %n'"
require_contains "$DEPLOYMENT_GUIDE" "/etc/charge-api.env"
require_contains "$DEPLOYMENT_GUIDE" "/var/lib/charge-api/charge_state.db"
require_contains "$DEPLOYMENT_GUIDE" "加密异地保存"

for service in "$CHARGE_SERVICE" "$BACKUP_SERVICE"; do
  require_contains "$service" "User=charge"
  require_contains "$service" "Group=charge"
  require_contains "$service" "NoNewPrivileges=true"
  require_contains "$service" "PrivateTmp=true"
  require_contains "$service" "ProtectSystem=strict"
  require_contains "$service" "ProtectHome=true"
  require_contains "$service" "ProtectControlGroups=true"
  require_contains "$service" "ProtectKernelTunables=true"
  require_contains "$service" "ProtectKernelModules=true"
  require_contains "$service" "RestrictNamespaces=true"
  require_contains "$service" "LockPersonality=true"
  require_contains "$service" "UMask=0077"
  require_contains "$service" "EnvironmentFile="
  require_contains "$service" "ExecStart="
  require_contains "$service" "ReadWritePaths=/var/lib/charge-api"
done

require_contains "$CHARGE_SERVICE" "StartLimitIntervalSec=60"
require_contains "$CHARGE_SERVICE" "StartLimitBurst=5"
require_contains "$CHARGE_SERVICE" "TimeoutStopSec=35"
require_contains "$CHARGE_SERVICE" "MemoryDenyWriteExecute=true"
