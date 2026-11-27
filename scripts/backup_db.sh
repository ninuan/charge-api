#!/usr/bin/env bash
# SQLite 在线备份：用 sqlite3 的 .backup 命令保证一致性（WAL 模式下服务
# 运行中执行也安全），gzip 压缩为 0600 文件并按天数滚动清理。
# 库内凭据均为 AES-256-GCM 密文，但备份仍应按敏感数据处理（见 README 备份策略）。

set -Eeuo pipefail

CHARGE_DB_FILE="${CHARGE_DB_FILE:-/var/lib/charge-api/charge_state.db}"
BACKUP_DIR="${BACKUP_DIR:-/var/lib/charge-api/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

usage() {
  cat <<'EOF'
用法: backup_db.sh [--help]

对 charge_state.db 做 SQLite 在线备份，
压缩为 0600 权限的 .db.gz，并按保留天数滚动清理旧备份。

环境变量（均可覆盖，生产环境可写入 /etc/charge-backup.env）：
  CHARGE_DB_FILE   主数据库，默认 /var/lib/charge-api/charge_state.db
  BACKUP_DIR       备份目录，默认 /var/lib/charge-api/backups
  RETENTION_DAYS   保留天数，默认 14
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

if ! command -v sqlite3 >/dev/null; then
  echo "缺少 sqlite3 命令，请先安装（Ubuntu: sudo apt install -y sqlite3）" >&2
  exit 1
fi
if [[ ! -f "$CHARGE_DB_FILE" ]]; then
  echo "找不到数据库文件: $CHARGE_DB_FILE" >&2
  exit 1
fi

if [[ ! "$RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "RETENTION_DAYS 必须是非负整数: $RETENTION_DAYS" >&2
  exit 1
fi
if [[ "$BACKUP_DIR" == *"'"* || "$BACKUP_DIR" == *$'\n'* ]]; then
  echo "BACKUP_DIR 包含非法字符: $BACKUP_DIR" >&2
  exit 1
fi

umask 077
db_owner=""
if [[ "$(id -u)" -eq 0 ]]; then
  if db_owner="$(stat -c '%u:%g' "$CHARGE_DB_FILE" 2>/dev/null)"; then
    :
  else
    db_owner=""
  fi
fi
mkdir -p "$BACKUP_DIR"
if [[ -n "$db_owner" ]]; then
  chown "$db_owner" "$BACKUP_DIR" 2>/dev/null || true
fi
stage_dir="$(mktemp -d "$BACKUP_DIR/.staging.XXXXXX")"

restore_sidecar_owner() {
  if [[ -n "$db_owner" ]]; then
    for sidecar in "${CHARGE_DB_FILE}-wal" "${CHARGE_DB_FILE}-shm"; do
      if [[ -e "$sidecar" ]]; then
        chown "$db_owner" "$sidecar" 2>/dev/null || true
      fi
    done
  fi
}

cleanup() {
  restore_sidecar_owner
  rm -rf "$stage_dir"
}
trap cleanup EXIT

stamp="$(date +%Y%m%d-%H%M%S)"
staged_db="$stage_dir/charge_state.db"
final_gz="$BACKUP_DIR/charge_state-${stamp}.db.gz"
if [[ -e "$final_gz" ]]; then
  final_gz="$BACKUP_DIR/charge_state-${stamp}-$$.db.gz"
fi

sqlite3 -cmd ".timeout 5000" "$CHARGE_DB_FILE" ".backup '$staged_db'"
restore_sidecar_owner

if [[ "$(sqlite3 "$staged_db" "PRAGMA integrity_check;")" != "ok" ]]; then
  echo "备份完整性校验失败: $CHARGE_DB_FILE" >&2
  exit 1
fi

gzip -f "$staged_db"
gzip -t "${staged_db}.gz"
chmod 600 "${staged_db}.gz"
if [[ -n "$db_owner" ]]; then
  chown "$db_owner" "${staged_db}.gz" 2>/dev/null || true
fi
mv "${staged_db}.gz" "$final_gz"
echo "已备份 $CHARGE_DB_FILE -> $final_gz"

find "$BACKUP_DIR" -maxdepth 1 -type f -name "*.db.gz" -mtime "+$RETENTION_DAYS" -delete
echo "已清理 ${RETENTION_DAYS} 天前的旧备份"
