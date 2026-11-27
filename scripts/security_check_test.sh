#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/security_check.sh"

if [[ ! -x "$SCRIPT" ]]; then
  echo "missing executable security check script: $SCRIPT"
  exit 1
fi

help_output="$("$SCRIPT" --help)"
for token in "CHARGE_BASE_URL" "CHARGE_LISTEN_PORT" "CHARGE_DB_FILE" "LOG_UNITS" "--dry-run"; do
  if [[ "$help_output" != *"$token"* ]]; then
    echo "help output should document $token"
    exit 1
  fi
done

dry_run_output="$(CHARGE_DB_FILE=/tmp/charge.db LOG_UNITS='charge-api' "$SCRIPT" --dry-run)"
for token in \
  "curl --silent --fail http://127.0.0.1:8080/healthz" \
  "ss -lntp | grep ':8080'" \
  "strings /tmp/charge.db" \
  "journalctl -u charge-api"; do
  if [[ "$dry_run_output" != *"$token"* ]]; then
    echo "dry-run output should include: $token"
    exit 1
  fi
done
