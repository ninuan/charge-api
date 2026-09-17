# yyb-go 凭据恢复修复（接续已安装的诊断补丁）

## 已确认的事实与边界

生产日志为 `fetch_login_buffer / upstream_rejected / -101`：刷新接口返回符合当前客户端校验的成功结果，但获取登录材料失败。旧逻辑只有两步都成功才保存新凭据，且把各种失败都归为 expired。这能确认凭据丢失路径，不能证明 -101 的全部原因或保证上游恢复成功。

本补丁基于本地 yyb_go 参考源码加 `yyb-recovery-diagnostics.patch`，适用于已安装该诊断补丁的源码；必须先 dry-run。未经校验不得强行套用到不同版本。只提供源码补丁，不包含凭据、资源目录或数据库。

## 行为

- 刷新成功后立即持久化新凭据，同时保留原登录材料并记录待完成标记；保存失败则停止，不继续调用上游。
- 获取登录材料失败后保留新凭据，下次使用尚未过期的新凭据重试该阶段；过期前 30 秒允许再次刷新。不自动循环重试，也不猜测 -101 为授权失效。
- 请求在刷新后被取消时，使用独立的最多 5 秒上下文尝试保存已轮换的凭据；数据库自身不可写时仍无法保证保存成功，但不会返回 alive。
- 登录材料和状态都保存成功才返回 alive；失败返回 recovery_failed。原始恢复凭据完全缺失仍返回 unknown。
- 同一微信身份的恢复和扫码保存共用有界分片锁；取得锁后重新读取数据库，避免使用等待前的旧凭据。锁仅覆盖同一服务进程，不支持多实例同时写同一数据库。
- Charge 配套代码识别 recovery_failed，返回 YYB_RECOVERY_FAILED 并保留绑定；更新先前缓存的 expired 状态，显示同步失败而非已过期。添加入口仅对未绑定账户直接引导扫码。
- 不新增已确认的微信永久失效错误码：暂无可靠映射；用户仍可主动重新扫码。

## 本地验证

- 在独立源码副本中 `go test -race ./...` 全部通过。
- 新测试覆盖第二步失败后的保存与重试、两次凭据写入失败、状态写入失败、请求取消后的保存、四个并发恢复请求、数据库重开后的恢复。
- 增量补丁在“原始源码 + 诊断补丁”副本中应用成功，逐文件比对与通过测试的源码完全一致。
- Charge 的 Go 全量测试与构建通过；最终前端 54 个文件、208 项单测，以及 45 项 Chromium 回归通过，eslint、TypeScript 与生产静态构建通过。全量检查中发现测试误用了 Playwright 的 `exact` 参数，已改为 Testing Library 支持的选项并重新验证。生产上游请求尚未验证。

## 服务器更新

先将本次 main 部署至 Charge，使服务器包含本目录的 `yyb-credential-recovery.patch`。也可单独上传补丁并调整下方路径。以下沿用已核实的 `/opt/yyb_go`、`yyb-go.service`。无需删除或重新绑定账户。

重启会短暂中断扫码服务，未完成的二维码会失效。此补丁不修改数据库结构，但会更新恢复凭据；不要为了回滚程序而恢复旧凭据数据库。

```bash
sudo bash <<'SH'
set -euo pipefail
cd /opt/yyb_go
patch_file=/opt/charge-api/scripts/patches/yyb-credential-recovery.patch
test -f "$patch_file"
test -f internal/httpapi/recovery_diagnostics.go
command -v go
command -v patch
patch --batch --forward --dry-run -p1 < "$patch_file"

backup_dir=$(mktemp -d /opt/yyb_go/recovery-backup.XXXXXX)
chmod 700 "$backup_dir"
cp -p yyb-go "$backup_dir/yyb-go"
tar -czf "$backup_dir/source.tar.gz" go.mod go.sum internal cmd
printf '程序与源码备份：%s\n' "$backup_dir"

patch --batch --forward -p1 < "$patch_file"
go test ./...
build_dir=$(mktemp -d /tmp/yyb-recovery-build.XXXXXX)
go build -o "$build_dir/yyb-go" ./cmd/yyb-go
install -m 0755 "$build_dir/yyb-go" /opt/yyb_go/yyb-go.recovery-new
mv /opt/yyb_go/yyb-go.recovery-new /opt/yyb_go/yyb-go
systemctl restart yyb-go
systemctl is-active yyb-go
printf '更新完成。备份目录：%s\n' "$backup_dir"
SH
```

任一步失败即停止并检查错误；不要重复强制应用补丁。如果程序不能正常启动，用输出的备份目录替换下面路径，恢复旧二进制（不恢复数据库）：

```bash
sudo install -m 0755 /opt/yyb_go/recovery-backup.替换为实际目录/yyb-go /opt/yyb_go/yyb-go.rollback
sudo mv /opt/yyb_go/yyb-go.rollback /opt/yyb_go/yyb-go
sudo systemctl restart yyb-go
```

源码备份保留用于排查或后续恢复，不会因二进制回滚而自动还原。测试失败发生在安装程序之前时，原服务仍运行，但源文件已经应用补丁。

## 生产验收

Charge 配套修改也需要正常构建部署；仅更新 yyb-go 时，旧 Charge 能拒绝失败结果，但可能仍显示旧过期状态或通用错误。

1. 不重新扫码，手动同步一次，记录成功或页面错误。若仍失败，只收集以下安全诊断，先不要连续重试：

```bash
sudo journalctl -u yyb-go --since '5 minutes ago' --no-pager -o cat \
  | grep -oE 'yyb_recovery stage=[a-z_]+ kind=[a-z_]+ upstream_code=[^[:space:]]+ http_status=[^[:space:]]+'
```

2. 如果仍返回 fetch_login_buffer / -101，说明本补丁没有解决上游拒绝原因，需要进一步核对请求协议；不要将其报告为已恢复。
3. 无论是否成功，已绑定用户添加入口都不应仅因缓存 expired/unknown 自动弹出扫码；恢复失败应保留桩号，并明确提示可重试。

原始本地 yyb_go 目录未改动。本补丁与 Charge 配套修改按用户授权合并至本地 main；服务器仍需按上述步骤更新。
