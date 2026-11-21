#!/usr/bin/env bash
# SQLite 在线备份：用 sqlite3 的 .backup 命令保证一致性（WAL 模式下服务
# 运行中执行也安全），gzip 压缩为 0600 文件并按天数滚动清理。
# 库内凭据均为 AES-256-GCM 密文，但备份仍应按敏感数据处理（见 README 备份策略）。

set -Eeuo pipefail

CHARGE_DB_FILE="${CHARGE_DB_FILE:-/var/lib/charge-api/charge_state.db}"
YYB_DB_FILE="${YYB_DB_FILE:-/opt/yyb_go/resource/db/yyb.db}"
BACKUP_DIR="${BACKUP_DIR:-/var/lib/charge-api/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

usage() {
  cat <<'EOF'
用法: backup_db.sh [--help]

对 charge_state.db（以及可读时的 yyb.db）做 SQLite 在线备份，
压缩为 0600 权限的 .db.gz，并按保留天数滚动清理旧备份。

环境变量（均可覆盖，生产环境可写入 /etc/charge-backup.env）：
  CHARGE_DB_FILE   主数据库，默认 /var/lib/charge-api/charge_state.db
  YYB_DB_FILE      扫码 sidecar 数据库，默认 /opt/yyb_go/resource/db/yyb.db（缺失时跳过）
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
mkdir -p "$BACKUP_DIR"
stage_dir="$(mktemp -d "$BACKUP_DIR/.staging.XXXXXX")"
trap 'rm -rf "$stage_dir"' EXIT
stamp="$(date +%Y%m%d-%H%M%S)"

backup_one() {
  local source="$1" name="$2"
  local owner=""
  if [[ "$(id -u)" -eq 0 ]]; then
    if owner="$(stat -c '%u:%g' "$source" 2>/dev/null)"; then
      :
    else
      owner=""
    fi
  fi
  local staged_db="$stage_dir/${name}.db"
  local final_gz="$BACKUP_DIR/${name}-${stamp}.db.gz"
  if [[ -e "$final_gz" ]]; then
    final_gz="$BACKUP_DIR/${name}-${stamp}-$$.db.gz"
  fi
  sqlite3 -cmd ".timeout 5000" "$source" ".backup '$staged_db'"
  if [[ -n "$owner" ]]; then
    for sidecar in "${source}-wal" "${source}-shm"; do
      if [[ -e "$sidecar" ]]; then
        chown "$owner" "$sidecar" 2>/dev/null || true
      fi
    done
  fi
  if [[ "$(sqlite3 "$staged_db" "PRAGMA integrity_check;")" != "ok" ]]; then
    echo "备份完整性校验失败: $source" >&2
    return 1
  fi
  gzip -f "$staged_db"
  gzip -t "${staged_db}.gz"
  chmod 600 "${staged_db}.gz"
  mv "${staged_db}.gz" "$final_gz"
  echo "已备份 $source -> $final_gz"
}

backup_one "$CHARGE_DB_FILE" "charge_state"
if [[ -r "$YYB_DB_FILE" ]]; then
  backup_one "$YYB_DB_FILE" "yyb"
else
  echo "跳过 yyb 数据库（$YYB_DB_FILE 不存在或不可读）"
fi

find "$BACKUP_DIR" -maxdepth 1 -type f -name "*.db.gz" -mtime "+$RETENTION_DAYS" -delete
echo "已清理 ${RETENTION_DAYS} 天前的旧备份"
