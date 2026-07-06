#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/security_check.sh"

if [[ ! -x "$SCRIPT" ]]; then
  echo "missing executable security check script: $SCRIPT"
  exit 1
fi

help_output="$($SCRIPT --help)"
for token in "YYB_BASE_URL" "CHARGE_BASE_URL" "YYB_DB_FILE" "CHARGE_DB_FILE" "LOG_UNITS" "--dry-run"; do
  if [[ "$help_output" != *"$token"* ]]; then
    echo "help output should document $token"
    exit 1
  fi
done

dry_run_output="$(YYB_DB_FILE=/tmp/yyb.db CHARGE_DB_FILE=/tmp/charge.db LOG_UNITS='charge-api yyb-go' "$SCRIPT" --dry-run)"
for token in \
  "curl -i http://127.0.0.1:8000/accounts" \
  "curl --silent --fail http://127.0.0.1:8080/healthz" \
  "ss -lntp" \
  "strings /tmp/yyb.db" \
  "strings /tmp/charge.db" \
  "journalctl -u charge-api -u yyb-go"; do
  if [[ "$dry_run_output" != *"$token"* ]]; then
    echo "dry-run output should include: $token"
    exit 1
  fi
done
