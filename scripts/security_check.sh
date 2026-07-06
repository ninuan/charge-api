#!/usr/bin/env bash

set -Eeuo pipefail

CHARGE_BASE_URL="${CHARGE_BASE_URL:-http://127.0.0.1:8080}"
YYB_BASE_URL="${YYB_BASE_URL:-http://127.0.0.1:8000}"
YYB_LISTEN_PORT="${YYB_LISTEN_PORT:-8000}"
CHARGE_DB_FILE="${CHARGE_DB_FILE:-/var/lib/charge-api/charge_state.db}"
YYB_DB_FILE="${YYB_DB_FILE:-/opt/yyb_go/resource/db/yyb.db}"
LOG_UNITS="${LOG_UNITS:-charge-api yyb-go}"
LOG_SINCE="${LOG_SINCE:-1 hour ago}"
CHARGE_SIGNED_FLOW_URL="${CHARGE_SIGNED_FLOW_URL:-}"
CHARGE_SESSION_COOKIE="${CHARGE_SESSION_COOKIE:-}"
YYB_SENSITIVE_REGEX="${YYB_SENSITIVE_REGEX:-login_buffer|accesstoken|refreshtoken|IQdV|wxopenid}"
CHARGE_SENSITIVE_REGEX="${CHARGE_SENSITIVE_REGEX:-wxopenid|info=|deviceid=|yyb-openid}"
LOG_SENSITIVE_REGEX="${LOG_SENSITIVE_REGEX:-accesstoken|refreshtoken|login_buffer|info=|wxopenid|Cookie:}"
DRY_RUN=0

usage() {
  cat <<USAGE
Usage: scripts/security_check.sh [--dry-run]

End-to-end security checks for the Charge + yyb_go loopback deployment.

Environment variables:
  CHARGE_BASE_URL          Charge URL, default: http://127.0.0.1:8080
  YYB_BASE_URL             yyb_go URL, default: http://127.0.0.1:8000
  YYB_LISTEN_PORT          yyb_go listen port, default: 8000
  CHARGE_DB_FILE           Charge SQLite file, default: /var/lib/charge-api/charge_state.db
  YYB_DB_FILE              yyb_go SQLite file, default: /opt/yyb_go/resource/db/yyb.db
  LOG_UNITS                systemd units, default: charge-api yyb-go
  LOG_SINCE                journalctl time window, default: 1 hour ago
  CHARGE_SIGNED_FLOW_URL   Optional Charge endpoint that triggers a signed yyb_go call
  CHARGE_SESSION_COOKIE    Optional Cookie header value for CHARGE_SIGNED_FLOW_URL
  YYB_SENSITIVE_REGEX      Regex used for yyb_go DB plaintext scan
  CHARGE_SENSITIVE_REGEX   Regex used for Charge DB plaintext scan
  LOG_SENSITIVE_REGEX      Regex used for journal log plaintext scan

Checks:
  1. Unsigned yyb_go /accounts request returns 401.
  2. Charge is reachable via /healthz, and optional signed flow URL succeeds.
  3. yyb_go listens on 127.0.0.1:8000 only, not 0.0.0.0:8000.
  4. yyb_go SQLite does not contain known plaintext tokens.
  5. Charge SQLite does not contain known plaintext Cookie or yyb binding values.
  6. Recent service logs do not contain sensitive values.
USAGE
}

log_step() {
  printf '\n==> %s\n' "$1"
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

print_cmd() {
  echo "[dry-run] $*"
}

run_or_print() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    print_cmd "$*"
  else
    bash -c "$*"
  fi
}

for arg in "$@"; do
  case "$arg" in
    --help|-h)
      usage
      exit 0
      ;;
    --dry-run)
      DRY_RUN=1
      ;;
    *)
      fail "unknown argument: $arg"
      ;;
  esac
done

log_step "Unsigned yyb_go requests are rejected"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "curl -i ${YYB_BASE_URL}/accounts"
else
  yyb_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "${YYB_BASE_URL}/accounts")"
  [[ "$yyb_status" == "401" ]] || fail "expected unsigned yyb_go /accounts to return 401, got $yyb_status"
  echo "OK: unsigned yyb_go /accounts returned 401"
fi

log_step "Charge API is reachable through Charge only"
run_or_print "curl --silent --fail ${CHARGE_BASE_URL}/healthz >/dev/null"
if [[ -n "$CHARGE_SIGNED_FLOW_URL" ]]; then
  if [[ "$DRY_RUN" -eq 1 ]]; then
    print_cmd "curl --silent --fail ${CHARGE_SIGNED_FLOW_URL} >/dev/null"
  elif [[ -n "$CHARGE_SESSION_COOKIE" ]]; then
    curl --silent --fail -H "Cookie: ${CHARGE_SESSION_COOKIE}" "$CHARGE_SIGNED_FLOW_URL" >/dev/null
    echo "OK: configured Charge signed-flow endpoint succeeded"
  else
    curl --silent --fail "$CHARGE_SIGNED_FLOW_URL" >/dev/null
    echo "OK: configured Charge signed-flow endpoint succeeded"
  fi
else
  echo "INFO: CHARGE_SIGNED_FLOW_URL is not set; checked /healthz only."
fi

log_step "yyb_go is bound to loopback only"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "ss -lntp | grep ':${YYB_LISTEN_PORT}'"
else
  listen_output="$(ss -lntp | grep ":${YYB_LISTEN_PORT}" || true)"
  [[ -n "$listen_output" ]] || fail "no listener found for port ${YYB_LISTEN_PORT}"
  echo "$listen_output"
  [[ "$listen_output" == *"127.0.0.1:${YYB_LISTEN_PORT}"* ]] || fail "yyb_go is not listening on 127.0.0.1:${YYB_LISTEN_PORT}"
  [[ "$listen_output" != *"0.0.0.0:${YYB_LISTEN_PORT}"* ]] || fail "yyb_go is exposed on 0.0.0.0:${YYB_LISTEN_PORT}"
  [[ "$listen_output" != *"[::]:${YYB_LISTEN_PORT}"* ]] || fail "yyb_go is exposed on [::]:${YYB_LISTEN_PORT}"
  echo "OK: yyb_go listener is loopback-only"
fi

log_step "yyb_go DB contains no known plaintext sensitive values"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "strings ${YYB_DB_FILE} | grep -E '${YYB_SENSITIVE_REGEX}'"
else
  [[ -f "$YYB_DB_FILE" ]] || fail "yyb DB file not found: $YYB_DB_FILE"
  if strings "$YYB_DB_FILE" | grep -E "$YYB_SENSITIVE_REGEX"; then
    fail "yyb_go DB contains plaintext sensitive values"
  fi
  echo "OK: yyb_go DB plaintext scan found no matches"
fi

log_step "Charge DB contains no known plaintext Cookie or yyb binding values"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "strings ${CHARGE_DB_FILE} | grep -E '${CHARGE_SENSITIVE_REGEX}'"
else
  [[ -f "$CHARGE_DB_FILE" ]] || fail "Charge DB file not found: $CHARGE_DB_FILE"
  if strings "$CHARGE_DB_FILE" | grep -E "$CHARGE_SENSITIVE_REGEX"; then
    fail "Charge DB contains plaintext sensitive values"
  fi
  echo "OK: Charge DB plaintext scan found no matches"
fi

log_step "Recent service logs contain no known sensitive values"
if [[ "$DRY_RUN" -eq 1 ]]; then
  read -r -a units <<< "$LOG_UNITS"
  cmd="journalctl"
  for unit in "${units[@]}"; do
    cmd+=" -u ${unit}"
  done
  cmd+=" --since '${LOG_SINCE}' | grep -E '${LOG_SENSITIVE_REGEX}'"
  print_cmd "$cmd"
else
  read -r -a units <<< "$LOG_UNITS"
  journal_args=()
  for unit in "${units[@]}"; do
    journal_args+=("-u" "$unit")
  done
  if journalctl "${journal_args[@]}" --since "$LOG_SINCE" | grep -E "$LOG_SENSITIVE_REGEX"; then
    fail "recent logs contain sensitive values"
  fi
  echo "OK: recent log plaintext scan found no matches"
fi

log_step "Security check finished"
