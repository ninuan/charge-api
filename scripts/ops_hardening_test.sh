#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT_DIR/README.md"
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
require_file "$CHARGE_SERVICE"
require_file "$YYB_SERVICE"

require_contains "$README" "## 生产运维加固"
require_contains "$README" "/opt/charge-api/backend/charge-server -listen 127.0.0.1:8080"
require_contains "$README" "/opt/yyb_go/yyb-go -host 127.0.0.1 -port 8000"
require_contains "$README" "ss -lntp | grep ':8000'"
require_contains "$README" "127.0.0.1:8000"
require_contains "$README" "stat -c '%a %n' /etc/charge-api.env /etc/yyb-go.env /var/lib/charge-api/charge_state.db /opt/yyb_go/resource/db/yyb.db"
require_contains "$README" "加密备份"
require_contains "$README" "8000/tcp"

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
