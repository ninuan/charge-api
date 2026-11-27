#!/usr/bin/env bash

set -Eeuo pipefail

CHARGE_BASE_URL="${CHARGE_BASE_URL:-http://127.0.0.1:8080}"
CHARGE_LISTEN_PORT="${CHARGE_LISTEN_PORT:-8080}"
CHARGE_DB_FILE="${CHARGE_DB_FILE:-/var/lib/charge-api/charge_state.db}"
LOG_UNITS="${LOG_UNITS:-charge-api}"
LOG_SINCE="${LOG_SINCE:-1 hour ago}"
CHARGE_SENSITIVE_REGEX="${CHARGE_SENSITIVE_REGEX:-wxopenid|info=|deviceid=|AT_[A-Za-z0-9]{16,}}"
LOG_SENSITIVE_REGEX="${LOG_SENSITIVE_REGEX:-info=|wxopenid|Cookie:|charge_session=|AT_[A-Za-z0-9]{16,}}"
DRY_RUN=0

usage() {
  cat <<USAGE
Usage: scripts/security_check.sh [--dry-run]

End-to-end security checks for the Charge Console loopback deployment.

Environment variables:
  CHARGE_BASE_URL          Charge URL, default: http://127.0.0.1:8080
  CHARGE_LISTEN_PORT       Charge listen port, default: 8080
  CHARGE_DB_FILE           Charge SQLite file, default: /var/lib/charge-api/charge_state.db
  LOG_UNITS                systemd units, default: charge-api
  LOG_SINCE                journalctl time window, default: 1 hour ago
  CHARGE_SENSITIVE_REGEX   Regex used for Charge DB plaintext scan
  LOG_SENSITIVE_REGEX      Regex used for journal log plaintext scan

Checks:
  1. Charge is reachable via /healthz.
  2. Charge listens on loopback (127.0.0.1 / ::1) only, not 0.0.0.0.
  3. Charge SQLite DB (+ WAL) does not contain known plaintext Cookie or token values.
  4. Recent service logs do not contain sensitive values.
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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-security-check.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

scan_file_no_matches() {
  local file="$1" regex="$2" label="$3"
  command -v strings >/dev/null 2>&1 || fail "missing required command: strings"
  [[ -f "$file" ]] || fail "${label} file not found: ${file}"
  [[ -r "$file" ]] || fail "${label} file is not readable: ${file}"
  local tmp_out="$TMP_DIR/scan.out"
  if ! strings "$file" > "$tmp_out"; then
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

validate_http_url "CHARGE_BASE_URL" "$CHARGE_BASE_URL"
if [[ ! "$CHARGE_LISTEN_PORT" =~ ^[0-9]+$ ]]; then
  fail "invalid CHARGE_LISTEN_PORT: ${CHARGE_LISTEN_PORT}"
fi

log_step "Charge API is reachable via /healthz"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "curl --silent --fail ${CHARGE_BASE_URL}/healthz >/dev/null"
else
  curl --silent --fail --max-time 10 "${CHARGE_BASE_URL}/healthz" >/dev/null
  echo "OK: ${CHARGE_BASE_URL}/healthz succeeded"
fi

log_step "Charge API is bound to loopback only"
if [[ "$DRY_RUN" -eq 1 ]]; then
  print_cmd "ss -lntp | grep ':${CHARGE_LISTEN_PORT}'"
else
  command -v ss >/dev/null 2>&1 || fail "missing required command: ss"
  listen_output="$(ss -H -lntp | grep -E ":${CHARGE_LISTEN_PORT}([[:space:]]|$)" || true)"
  [[ -n "$listen_output" ]] || fail "no listener found for port ${CHARGE_LISTEN_PORT}"
  echo "$listen_output"
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    local_addr="$(awk '{print $4}' <<< "$line")"
    case "$local_addr" in
      "127.0.0.1:${CHARGE_LISTEN_PORT}"|"[::1]:${CHARGE_LISTEN_PORT}"|"::1:${CHARGE_LISTEN_PORT}")
        ;;
      *)
        fail "Charge API listener is exposed on non-loopback address: ${local_addr}"
        ;;
    esac
  done <<< "$listen_output"
  echo "OK: Charge API listener is loopback-only"
fi

log_step "Charge DB contains no known plaintext Cookie or token values"
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
  tmp_logs="$TMP_DIR/journal.out"
  if ! journalctl "${journal_args[@]}" --since "$LOG_SINCE" > "$tmp_logs"; then
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
