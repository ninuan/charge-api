#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/dev_process.sh"

# Mock process inspection so foreign/active processes are never signalled by tests.
run_case() (
  local scenario="$1"
  OCCUPIED=222
  SERVICE=backend
  PROCESS_CWD="$ROOT_DIR/backend"
  PROCESS_COMMAND=/tmp/go-build/example/server
  PROCESS_OWNER="$UID"
  SIGNALS=""
  LIVE=false
  STUBBORN=false
  REUSED=false
  INSPECTABLE=true
  case "$scenario" in
    free) OCCUPIED="" ;;
    frontend) SERVICE=frontend; PROCESS_CWD="$ROOT_DIR/frontend"; PROCESS_COMMAND='next-server (v16.2.12)' ;;
    foreign) PROCESS_CWD=/another/checkout/backend ;;
    unrelated) PROCESS_COMMAND=/usr/bin/python ;;
    owner) PROCESS_OWNER=$((UID + 1)) ;;
    active) LIVE=true ;;
    stubborn) STUBBORN=true ;;
    reused) STUBBORN=true; REUSED=true ;;
    uninspectable) INSPECTABLE=false ;;
    stale) ;;
    linux) PROCESS_COMMAND=server ;;
    *) exit 1 ;;
  esac
  lsof() {
    if [[ " $* " == *" -d cwd "* ]]; then
      printf 'p222\nfcwd\nn%s\n' "$PROCESS_CWD"
    elif [[ -n "$OCCUPIED" ]]; then
      printf '%s\n' "$OCCUPIED"
    else return 1
    fi
  }
  ps() {
    $INSPECTABLE || return 1
    [[ -n "$OCCUPIED" ]] || return 1
    case "$4" in
      uid=) echo "$PROCESS_OWNER" ;;
      comm=) echo "$PROCESS_COMMAND" ;;
      ppid=) if [[ "$2" == 222 ]]; then echo 333; else echo 1; fi ;;
      args=) if $LIVE; then echo 'bash scripts/dev.sh'; else echo 'go run ./cmd/server'; fi ;;
      lstart=) if $REUSED && [[ -n "$SIGNALS" ]]; then echo new; else echo old; fi ;;
      *) return 1 ;;
    esac
  }
  kill() {
    if [[ "$1" == -0 ]]; then [[ -n "$OCCUPIED" ]]; return; fi
    SIGNALS="$SIGNALS $1"
    if ! $STUBBORN || [[ "$1" == -KILL ]]; then OCCUPIED=""; fi
  }
  sleep() { :; }
  result=0
  dev_prepare_port 8080 "$SERVICE" >/dev/null || result=$?
  case "$scenario" in
    foreign|unrelated|owner|active|uninspectable)
      [[ "$result" == 1 && -z "$SIGNALS" ]] ;;
    free) [[ "$result" == 0 && -z "$SIGNALS" ]] ;;
    stubborn) [[ "$result" == 0 && "$SIGNALS" == ' -TERM -KILL' ]] ;;
    reused) [[ "$result" == 1 && "$SIGNALS" == ' -TERM' ]] ;;
    *) [[ "$result" == 0 && "$SIGNALS" == ' -TERM' ]] ;;
  esac
)

for scenario in free stale linux frontend foreign unrelated owner active stubborn reused uninspectable; do
  if ! run_case "$scenario"; then
    echo "FAIL: dev port handling ($scenario)"
    exit 1
  fi
done

# Exercise real process-group cleanup, including an already-exited wrapper.
# All processes below are created by this test, with no network or app data.
set -m
group_pid=""
TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/charge-dev-process.XXXXXX")"
trap 'dev_stop_groups "$group_pid"; rm -rf "$TEST_DIR"' EXIT
(
  trap '' TERM
  sleep 60 &
  echo ready >"$TEST_DIR/ready"
  wait
) &
group_pid=$!
for ((attempt = 0; attempt < 50; attempt++)); do
  [[ -f "$TEST_DIR/ready" ]] && break
  sleep 0.1
done
[[ -f "$TEST_DIR/ready" ]]
dev_stop_groups "$group_pid"
wait "$group_pid" 2>/dev/null || true
if kill -0 -- "-$group_pid" 2>/dev/null; then
  echo "FAIL: development child process group survived cleanup"
  exit 1
fi
group_pid=""
(
  sleep 60 &
) &
group_pid=$!
wait "$group_pid"
dev_stop_groups "$group_pid"
if kill -0 -- "-$group_pid" 2>/dev/null; then
  echo "FAIL: orphaned child process group survived cleanup"
  exit 1
fi
group_pid=""
echo "dev process tests passed (11 port cases + 2 process-group cases)"
