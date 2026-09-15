# YYB 凭据恢复现场诊断补丁

## 已确认的证据

2026-09-14 22:16:44、22:17:01、22:18:25（北京时间）均为：

`yyb_get_code_failed (409) → yyb_account_refresh_expired → credential_sync_failed (409)`。

失败尚未进入 Mocele 登录。`expired` 是扫码服务的返回状态，不等于已经证实微信刷新令牌失效。本机参考实现 `refreshLiveness` 会吞掉刷新异常，将各种原因统一标为 expired；旧日志无法补回已丢弃的异常。

参考实现还存在两个待核实风险：令牌刷新成功后，获取 login_buffer 失败会丢弃新令牌；写入凭据的数据库错误被忽略。这些是代码风险，尚未证实为本次生产故障根因。

## 补丁范围和验证

`yyb-recovery-diagnostics.patch` 作用于 **yyb_go 源码**，不是 Charge 源码。

- 区分 `load_credentials`、`refresh_credentials`、`fetch_login_buffer`、`persist_credentials`。
- 只输出固定错误类别、数字业务码和 HTTP 状态。不会输出原始异常、响应正文、账号身份、Cookie 或令牌。
- 本补丁仅补充诊断，不改变令牌恢复策略，不宣称修复长期失效。
- 已在本机参考源码的独立副本通过 `go test ./...`，包括错误阶段测试和敏感字段不泄漏测试；原始源码上的 `patch --dry-run` 通过。
- 原始目录 `/Users/wang/Documents/go-project/yyb_go` 未修改。生产源码版本仍以服务器为准；补丁检查失败时停止，不强制应用。

## 在服务器复现一次

1. 将同目录补丁上传至服务器 `/tmp/yyb-recovery-diagnostics.patch`。
2. 以下按已有 systemd 示例的源码/程序目录 `/opt/yyb_go`、服务名 `yyb-go` 操作。如目录没有 `go.mod`，不要继续，需在实际源码目录构建后替换二进制。无需重新绑定。
3. 应用、测试和构建；任何命令失败即停止：

```bash
bash <<'SH'
set -eu
cd /opt/yyb_go
test -f go.mod
test -f internal/httpapi/app.go
patch --dry-run -p1 < /tmp/yyb-recovery-diagnostics.patch
sudo patch -p1 < /tmp/yyb-recovery-diagnostics.patch
go test ./...
yyb_build_dir=$(mktemp -d /tmp/yyb-recovery-build.XXXXXX)
go build -o "$yyb_build_dir/yyb-go" ./cmd/yyb-go
yyb_backup="/opt/yyb_go/yyb-go.before-diagnostics-$(date +%Y%m%d%H%M%S)"
sudo cp -p /opt/yyb_go/yyb-go "$yyb_backup"
sudo install -m 0755 "$yyb_build_dir/yyb-go" /opt/yyb_go/yyb-go.diagnostics-new
sudo mv /opt/yyb_go/yyb-go.diagnostics-new /opt/yyb_go/yyb-go
sudo systemctl restart yyb-go
sudo systemctl is-active yyb-go
printf '原程序备份：%s\n' "$yyb_backup"
SH
```

重启会短暂中断扫码服务并使未完成的二维码失效；已保存的绑定和数据库不删除。如果重启失败，可用上面的备份程序替换回去后重启服务。

4. 在 Charge 中点击一次“同步凭据”，然后执行：

```bash
sudo journalctl -u yyb-go --since "10 minutes ago" --no-pager -o cat \
  | grep 'yyb_recovery stage='
```

只需返回这些 `yyb_recovery` 行。不要粘贴环境文件、数据库、原始远端响应或完整服务日志。

## 如何解释下一份结果

- `refresh_credentials / missing_refresh_token`：保存的恢复凭据缺少刷新令牌。
- `refresh_credentials / upstream_rejected`：远端拒绝刷新；根据数字业务码继续核实原因。
- `fetch_login_buffer / ...`：令牌刷新已经成功，失败发生在取登录材料阶段，应优先处理新令牌持久化问题。
- `persist_credentials / storage`：检查数据库写入及权限；当前实现可能在保存失败后仍报 alive。
- `timeout / network / http_error / invalid_response`：不能按真实账号过期处理，继续检查对应请求阶段。

在取得新日志前，尚不能判断重新绑定是必须操作，或确定是哪一种服务端缺陷导致本次故障。
