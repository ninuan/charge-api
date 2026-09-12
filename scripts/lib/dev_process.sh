#!/usr/bin/env bash

# Only reclaim listeners belonging to this checkout and current user. Never
# terminate an arbitrary process just because it occupies a development port.
dev_listener_owned() {
  local pid="$1" service="$2" cwd command owner
  owner="$(ps -p "$pid" -o uid= 2>/dev/null)" || return 1
  [[ "${owner//[[:space:]]/}" == "$UID" ]] || return 1
  cwd="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p')"
  command="$(ps -p "$pid" -o comm= 2>/dev/null)" || return 1
  case "$service" in
    backend) [[ "$cwd" == "$ROOT_DIR/backend" && ( "$command" == */server || "$command" == server ) ]] ;;
    frontend) [[ "$cwd" == "$ROOT_DIR/frontend" && "$command" == *next-server* ]] ;;
    *) return 1 ;;
  esac
}

dev_has_live_launcher() {
  local pid="$1" command parent count
  for ((count = 0; count < 128; count++)); do
    parent="$(ps -p "$pid" -o ppid= 2>/dev/null)" || return 0
    parent="${parent//[[:space:]]/}"
    [[ "$parent" =~ ^[0-9]+$ ]] || return 0
    [[ "$parent" -gt 1 ]] || return 1
    command="$(ps -p "$parent" -o args= 2>/dev/null)" || return 0
    if [[ "$command" =~ (^|[[:space:]/])dev\.sh([[:space:]]|$) ]]; then
      return 0
    fi
    pid="$parent"
  done
  # An ancestry we cannot resolve safely must not be reclaimed.
  return 0
}

dev_prepare_port() {
  local port="$1" service="$2" pids pid identity current attempt
  pids="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -u)" || true
  [[ -n "$pids" ]] || return 0
  # Validate every listener before stopping any of them (IPv4/IPv6 can differ).
  for pid in $pids; do
    if ! dev_listener_owned "$pid" "$service"; then
      echo "端口 $port 被非本项目开发服务占用（PID ${pid}），未自动停止。请关闭该服务或修改端口。"
      return 1
    fi
    if dev_has_live_launcher "$pid"; then
      echo "端口 $port 的开发服务仍由另一个 dev.sh 管理（PID ${pid}）。请先在原终端按 Ctrl+C。"
      return 1
    fi
  done
  for pid in $pids; do
    identity="$(ps -p "$pid" -o lstart= 2>/dev/null)" || continue
    dev_listener_owned "$pid" "$service" || return 1
    echo "清理上次遗留的 $service 服务（端口 ${port}，PID ${pid}）..."
    kill -TERM "$pid" 2>/dev/null || true
    for ((attempt = 0; attempt < 50; attempt++)); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.1
    done
    current="$(ps -p "$pid" -o lstart= 2>/dev/null)" || current=""
    if [[ -n "$identity" && "$current" == "$identity" ]] && dev_listener_owned "$pid" "$service"; then
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
  # Killing is asynchronous; do not race the next bind against socket teardown.
  for ((attempt = 0; attempt < 50; attempt++)); do
    if ! lsof -nP -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  echo "端口 $port 仍被占用，无法启动，请检查对应进程。"
  return 1
}

dev_stop_groups() {
  local pid attempt alive
  for pid in "$@"; do
    [[ -n "$pid" ]] || continue
    kill -TERM -- "-$pid" 2>/dev/null || true
  done
  for ((attempt = 0; attempt < 50; attempt++)); do
    alive=false
    for pid in "$@"; do
      [[ -n "$pid" ]] || continue
      if kill -0 -- "-$pid" 2>/dev/null; then alive=true; fi
    done
    $alive || return 0
    sleep 0.1
  done
  for pid in "$@"; do
    [[ -n "$pid" ]] || continue
    kill -KILL -- "-$pid" 2>/dev/null || true
  done
}
