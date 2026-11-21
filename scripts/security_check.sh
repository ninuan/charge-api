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

validate_http_url() {
  local name="$1" value="$2"
  if [[ ! "$value" =~ ^https?://[^[:space:]\;\|\&\$\`\'\"\\]+$ ]]; then
    fail "invalid URL in ${name}: ${value}"
  fi
}

scan_file_no_matches() {
  local file="$1" regex="$2" label="$3"
  command -v strings >/dev/null 2>&1 || fail "missing required command: strings"
  [[ -f "$file" ]] || fail "${label} file not found: ${file}"
  [[ -r "$file" ]] || fail "${label} file is not readable: ${file}"
  local tmp_out
  tmp_out="$(mktemp)"
  if ! strings "$file" > "$tmp_out"; then
    rm -f "$tmp_out"
    fail "failed to scan ${label}: ${file}"
  fi
  local grep_status=0
  grep -qE -- "$regex" "$tmp_out" || grep_status=$?
  rm -f "$tmp_out"
  if [[ "$grep_status" -eq 0 ]]; then
    fail "${label} contains plaintext sensitive values: ${file}"
  elif [[ "$grep_status" -gt 1 ]]; then
    fail "grep failed while scanning ${label}: ${file}"
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

validate_http_url "YYB_BASE_URL" "$YYB_BASE_URL"
validate_http_url "CHARGE_BASE_URL" "$CHARGE_BASE_URL"
if [[ -n "$CHARGE_SIGNED_FLOW_URL" ]]; then
  validate_http_url "CHARGE_SIGNED_FLOW_URL" "$CHARGE_SIGNED_FLOW_URL"
fi
if [[ ! "$YYB_LISTEN_PORT" =~ ^[0-9]+$ ]]; then
  fail "invalid YYB_LISTEN_PORT: ${YYB_LISTEN_PORT}"
fi

log_step "Unsigned yyb_go requests are rejected"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "curl -i ${YYB_BASE_URL}/accounts"
else
  yyb_status="$(curl --silent --max-time 10 --output /dev/null --write-out '%{http_code}' "${YYB_BASE_URL}/accounts")"
  [[ "$yyb_status" == "401" ]] || fail "expected unsigned yyb_go /accounts to return 401, got $yyb_status"
  echo "OK: unsigned yyb_go /accounts returned 401"
fi

log_step "Charge API is reachable through Charge only"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "curl --silent --fail ${CHARGE_BASE_URL}/healthz >/dev/null"
else
  curl --silent --fail --max-time 10 "${CHARGE_BASE_URL}/healthz" >/dev/null
fi
if [[ -n "$CHARGE_SIGNED_FLOW_URL" ]]; then
  if [[ "$DRY_RUN" -eq 1 ]]; then
    print_cmd "curl --silent --fail ${CHARGE_SIGNED_FLOW_URL} >/dev/null"
  elif [[ -n "$CHARGE_SESSION_COOKIE" ]]; then
    printf 'header = "Cookie: %s"\n' "$CHARGE_SESSION_COOKIE" | \
      curl --silent --fail --max-time 10 --config - "$CHARGE_SIGNED_FLOW_URL" >/dev/null
    echo "OK: configured Charge signed-flow endpoint succeeded"
  else
    curl --silent --fail --max-time 10 "$CHARGE_SIGNED_FLOW_URL" >/dev/null
    echo "OK: configured Charge signed-flow endpoint succeeded"
  fi
else
  echo "INFO: CHARGE_SIGNED_FLOW_URL is not set; checked /healthz only."
fi

log_step "yyb_go is bound to loopback only"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "ss -lntp | grep ':${YYB_LISTEN_PORT}'"
else
  command -v ss >/dev/null 2>&1 || fail "missing required command: ss"
  listen_output="$(ss -H -lntp | grep -E ":${YYB_LISTEN_PORT}([[:space:]]|$)" || true)"
  [[ -n "$listen_output" ]] || fail "no listener found for port ${YYB_LISTEN_PORT}"
  echo "$listen_output"
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    local_addr="$(awk '{print $4}' <<< "$line")"
    case "$local_addr" in
      "127.0.0.1:${YYB_LISTEN_PORT}"|"[::1]:${YYB_LISTEN_PORT}"|"::1:${YYB_LISTEN_PORT}")
        ;;
      *)
        fail "yyb_go listener is exposed on non-loopback address: ${local_addr}"
        ;;
    esac
  done <<< "$listen_output"
  echo "OK: yyb_go listener is loopback-only"
fi

log_step "yyb_go DB contains no known plaintext sensitive values"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "strings ${YYB_DB_FILE} | grep -qE '${YYB_SENSITIVE_REGEX}'"
else
  scan_file_no_matches "$YYB_DB_FILE" "$YYB_SENSITIVE_REGEX" "yyb_go DB"
  if [[ -f "${YYB_DB_FILE}-wal" ]]; then
    scan_file_no_matches "${YYB_DB_FILE}-wal" "$YYB_SENSITIVE_REGEX" "yyb_go DB WAL"
  fi
  echo "OK: yyb_go DB plaintext scan found no matches"
fi

log_step "Charge DB contains no known plaintext Cookie or yyb binding values"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "strings ${CHARGE_DB_FILE} | grep -qE '${CHARGE_SENSITIVE_REGEX}'"
else
  scan_file_no_matches "$CHARGE_DB_FILE" "$CHARGE_SENSITIVE_REGEX" "Charge DB"
  if [[ -f "${CHARGE_DB_FILE}-wal" ]]; then
    scan_file_no_matches "${CHARGE_DB_FILE}-wal" "$CHARGE_SENSITIVE_REGEX" "Charge DB WAL"
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
  cmd+=" --since '${LOG_SINCE}' | grep -qE '${LOG_SENSITIVE_REGEX}'"
  print_cmd "$cmd"
else
  command -v journalctl >/dev/null 2>&1 || fail "missing required command: journalctl"
  read -r -a units <<< "$LOG_UNITS"
  journal_args=()
  for unit in "${units[@]}"; do
    journal_args+=("-u" "$unit")
  done
  tmp_logs="$(mktemp)"
  if ! journalctl "${journal_args[@]}" --since "$LOG_SINCE" > "$tmp_logs"; then
    rm -f "$tmp_logs"
    fail "failed to read service logs via journalctl"
  fi
  grep_status=0
  grep -qE -- "$LOG_SENSITIVE_REGEX" "$tmp_logs" || grep_status=$?
  rm -f "$tmp_logs"
  if [[ "$grep_status" -eq 0 ]]; then
    fail "recent logs contain sensitive values"
  elif [[ "$grep_status" -gt 1 ]]; then
    fail "grep failed while scanning recent logs"
  fi
  echo "OK: recent log plaintext scan found no matches"
fi

log_step "Security check finished"
