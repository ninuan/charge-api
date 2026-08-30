#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT_DIR/README.md"
DEPLOYMENT_GUIDE="$ROOT_DIR/docs/deployment.md"
CHARGE_SERVICE="$ROOT_DIR/deploy/systemd/charge.service"
YYB_SERVICE="$ROOT_DIR/deploy/systemd/yyb-go.service"

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
require_file "$YYB_SERVICE"

require_contains "$README" "[生产部署与运维指南](docs/deployment.md)"
require_contains "$DEPLOYMENT_GUIDE" "## 安全加固"
require_contains "$DEPLOYMENT_GUIDE" "/opt/charge-api/backend/charge-server"
require_contains "$DEPLOYMENT_GUIDE" "-listen 127.0.0.1:8080"
require_contains "$DEPLOYMENT_GUIDE" "/opt/yyb_go/yyb-go"
require_contains "$DEPLOYMENT_GUIDE" "127.0.0.1:8000"
require_contains "$DEPLOYMENT_GUIDE" "ss -lntp | grep -E ':8080|:8000'"
require_contains "$DEPLOYMENT_GUIDE" "stat -c '%a %n'"
require_contains "$DEPLOYMENT_GUIDE" "/etc/charge-api.env"
require_contains "$DEPLOYMENT_GUIDE" "/var/lib/charge-api/charge_state.db"
require_contains "$DEPLOYMENT_GUIDE" "加密异地保存"
require_contains "$DEPLOYMENT_GUIDE" "8000/tcp"

for service in "$CHARGE_SERVICE" "$YYB_SERVICE"; do
  require_contains "$service" "NoNewPrivileges=true"
  require_contains "$service" "PrivateTmp=true"
  require_contains "$service" "ProtectSystem=strict"
  require_contains "$service" "ProtectHome=true"
  require_contains "$service" "UMask=0077"
  require_contains "$service" "EnvironmentFile="
  require_contains "$service" "ExecStart="
done

require_contains "$CHARGE_SERVICE" "ReadWritePaths=/var/lib/charge-api"
require_contains "$YYB_SERVICE" "ReadWritePaths=/opt/yyb_go/resource"
require_contains "$YYB_SERVICE" "-host 127.0.0.1 -port 8000"
