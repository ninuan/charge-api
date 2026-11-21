# Charge Console v1.5.5 发布说明

本版本为安全、并发与运维可靠性加固版本，系统性修复了全栈代码审查中发现的高危与中危隐患，不涉及破坏性变更或数据库表结构迁移。

## 主要修复

### 后端安全与并发隔离
- **跨用户刷新隔离**：共享后台刷新航班不再把发起方的凭据过期、配额耗尽、退避或禁用错误传播给其他守候同一充电桩的用户。
- **消除全局锁阻塞与数据竞争**：`ChangePassword` 的 Argon2id 移出全局写锁并接入 `AuthGuard` 锁定；`UserRuntime.refresh` 释放锁后再执行上游 HTTP 请求；修复 `lastRemoteFetch` 指针逃逸导致的数据竞争；SSE 支持随 `SIGTERM` 立即优雅退出。
- **认证与会话加固**：限制登录/注册/改密并发 Argon2id 槽位防内存 OOM 与并发绕过；服务端强制校验 `MustChangePassword`；修复 `DeleteOthers` 与重启后会话上限淘汰导致的「幽灵会话」；`AuthGuard.RecordSuccess` 按单账号失败数精确回退 IP 计数。
- **配额、解析与上游通信加固**：修复临时提醒取消/完成重置当日配额与首次检查绕过配额问题；规范化 HTTP/2 小写抓包头；限制上游 `opennum <= 64`；禁止 `charger` 客户端自动跟随重定向并锁定 `mocele` 同域 HTTPS 重定向；`randomID` 改用 `crypto/rand`；解密前校验 nonce 长度。
- **接口与租户细节**：新增单用户 QR 会话限额与 WxPusher 测试重查 5 秒冷却；修复 `AddPileWithYYB` 前置校验、`RegisterUser`/`DeleteInvite` 竞态、禁用用户 SSE channel 泄露、CORS 缺失 `PUT`、静态目录列表暴露、管理员误清 Cookie 及 WxPusher 免打扰态误计故障。

### 前端状态与运维部署加固
- **前端**：修复 `EventSource` 进入 `CLOSED` 后 `streamRef` 未清空导致无法重连、通知标记已读并发双减 `unreadCount`，以及 `request()` 未在 `finally` 移除外部 `AbortSignal` 监听的问题。
- **安全检查与备份脚本**：`security_check.sh` 移除 `bash -c`、Cookie 改由标准输入传递、补扫 `-wal` 文件且不再静默假通过；`backup_db.sh` 增加临时暂存、完整性校验、`.timeout 5000` 并保护 `-wal`/`-shm` 属主。
- **原子部署与自动回滚**：`deploy.sh` 与 `deploy_git.sh` 改为前后端全部编译成功后再切换线上文件，健康检查核验 `systemctl is-active` 与 `/healthz` 状态，失败自动回滚至 `charge-server.prev`。

## 升级说明

1. 前后端同步更新至 `1.5.5`，无需数据库迁移或新增环境变量。
2. 更新后检查 `/healthz` 返回版本为 `1.5.5`。
