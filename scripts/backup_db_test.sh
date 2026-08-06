#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_SCRIPT="$ROOT_DIR/scripts/backup_db.sh"

help_output="$("$BACKUP_SCRIPT" --help)"
if [[ "$help_output" != *"RETENTION_DAYS"* ]] || [[ "$help_output" != *"BACKUP_DIR"* ]]; then
  echo "backup help 应说明 BACKUP_DIR 与 RETENTION_DAYS"
  exit 1
fi

# systemd 单元与主服务保持同等加固
BACKUP_SERVICE="$ROOT_DIR/deploy/systemd/charge-backup.service"
BACKUP_TIMER="$ROOT_DIR/deploy/systemd/charge-backup.timer"
for needle in "NoNewPrivileges=true" "PrivateTmp=true" "ProtectSystem=strict" "ProtectHome=true" "UMask=0077" "EnvironmentFile=" "ExecStart=" "ReadWritePaths=/var/lib/charge-api"; do
  if ! grep -q "$needle" "$BACKUP_SERVICE"; then
    echo "charge-backup.service 缺少 $needle"
    exit 1
  fi
done
for needle in "OnCalendar=" "Persistent=true" "WantedBy=timers.target"; do
  if ! grep -q "$needle" "$BACKUP_TIMER"; then
    echo "charge-backup.timer 缺少 $needle"
    exit 1
  fi
done

if ! command -v sqlite3 >/dev/null; then
  echo "跳过备份功能测试：本机没有 sqlite3"
  exit 0
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

sqlite3 "$workdir/charge_state.db" "CREATE TABLE t(x); INSERT INTO t VALUES (1);"

# 过期备份应被滚动清理
mkdir -p "$workdir/backups"
old_backup="$workdir/backups/charge_state-20200101-000000.db.gz"
touch "$old_backup"
touch -t 202001010000 "$old_backup"

CHARGE_DB_FILE="$workdir/charge_state.db" \
YYB_DB_FILE="$workdir/missing-yyb.db" \
BACKUP_DIR="$workdir/backups" \
RETENTION_DAYS=1 \
  "$BACKUP_SCRIPT" >/dev/null

latest="$(find "$workdir/backups" -name "charge_state-*.db.gz" -newer "$workdir/charge_state.db" | head -1 || true)"
if [[ -z "$latest" ]]; then
  latest="$(ls "$workdir/backups"/charge_state-*.db.gz 2>/dev/null | head -1)"
fi
if [[ -z "$latest" ]]; then
  echo "应产出 charge_state 备份文件"
  exit 1
fi

# GNU stat 用 -c，BSD/macOS 用 -f；注意 GNU 的 -f 是"文件系统信息"且不报错，
# 因此必须先探测 -c 是否可用，不能反过来回退。
if stat -c '%a' "$latest" >/dev/null 2>&1; then
  perms="$(stat -c '%a' "$latest")"
else
  perms="$(stat -f '%Lp' "$latest")"
fi
if [[ "$perms" != "600" ]]; then
  echo "备份文件权限应为 600，实际 $perms"
  exit 1
fi

if [[ -f "$old_backup" ]]; then
  echo "过期备份应被清理"
  exit 1
fi

if ! gunzip -t "$latest"; then
  echo "备份 gzip 校验失败"
  exit 1
fi

# 发布门禁不仅检查压缩包，还要证明它能恢复为可读且完整的 SQLite 数据库。
restored="$workdir/restored.db"
gzip -dc "$latest" > "$restored"
if [[ "$(sqlite3 "$restored" "PRAGMA integrity_check;")" != "ok" ]]; then
  echo "恢复后的数据库完整性检查失败"
  exit 1
fi
if [[ "$(sqlite3 "$restored" "SELECT x FROM t;")" != "1" ]]; then
  echo "恢复后的数据库数据不一致"
  exit 1
fi

echo "backup_db 检查通过"
