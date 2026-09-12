"use client"

import {
  ArrowLeftIcon,
  ChevronRightIcon,
  CopyIcon,
  KeyRoundIcon,
  RefreshCwIcon,
  Trash2Icon,
  UsersIcon,
} from "lucide-react"
import { useRouter, useSearchParams } from "next/navigation"
import { useCallback, useEffect, useState } from "react"
import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"
import { AdminUserFilters } from "@/components/admin-user-filters"
import { AdminUserRefreshTiming } from "@/components/admin-user-refresh-timing"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogDescription,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { adminApi } from "@/lib/admin-api"
import { useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import { formatSnapshotTime, updatePageQuery } from "@/lib/workbench"
import type {
  AdminUserDetail,
  AdminUserListQuery,
  AdminUserPage,
  CredentialState,
  UserRole,
} from "@/lib/types"
import {
  SectionHeading,
  StatusPill,
  WorkbenchButton,
  WorkbenchEmpty,
  WorkbenchError,
  WorkbenchLoading,
} from "./surfaces"

const credentialLabels: Record<CredentialState, string> = {
  unbound: "尚未绑定",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "同步失败",
  expired: "登录已失效",
}
function userQuery(params: URLSearchParams): AdminUserListQuery {
  const credential = params.get("credential")
  return {
    page: Math.max(1, Number(params.get("page")) || 1),
    pageSize: 15,
    search: params.get("search") ?? "",
    account:
      params.get("account") === "enabled"
        ? "enabled"
        : params.get("account") === "disabled"
          ? "disabled"
          : "all",
    credential:
      credential && Object.hasOwn(credentialLabels, credential)
        ? (credential as CredentialState)
        : "all",
    health:
      params.get("health") === "risk"
        ? "risk"
        : params.get("health") === "healthy"
          ? "healthy"
          : "all",
  }
}

export function WorkbenchAdminUsers({
  currentUserId,
}: {
  currentUserId?: string
}) {
  const params = useSearchParams()
  const key = params.toString(),
    selectedId = params.get("user")
  const query = userQuery(new URLSearchParams(key))
  const [page, setPage] = useState<AdminUserPage | null>(null)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    if (selectedId) return
    let active = true
    adminApi
      .users(userQuery(new URLSearchParams(key)))
      .then((data) => {
        if (active) {
          setPage(data)
          setError("")
        }
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error ? reason.message : "用户列表暂时不可用。"
          )
      })
    return () => {
      active = false
    }
  }, [key, selectedId, revision])
  const apply = async (next: AdminUserListQuery = query) => {
    updatePageQuery({
      tab: "users",
      user: null,
      page: next.page === 1 ? null : next.page,
      search: next.search || null,
      account: next.account === "all" ? null : next.account,
      credential: next.credential === "all" ? null : next.credential,
      health: next.health === "all" ? null : next.health,
    })
    setRevision((value) => value + 1)
  }
  if (selectedId)
    return (
      <WorkbenchAdminUserDetail
        key={selectedId}
        userId={selectedId}
        currentUserId={currentUserId}
      />
    )
  return (
    <div className="wb-admin-users">
      <div className="wb-filter-panel wb-continuous">
        <AdminUserFilters key={key} query={query} onApply={apply} />
      </div>
      {error ? (
        <WorkbenchError
          message={error}
          retry={() => setRevision((value) => value + 1)}
        />
      ) : !page ? (
        <WorkbenchLoading label="正在读取用户" />
      ) : !page.items.length ? (
        <WorkbenchEmpty
          icon={UsersIcon}
          title="没有匹配的用户"
          description="换个用户名，或清除筛选再试。"
          action={
            <WorkbenchButton
              variant="outline"
              onClick={() =>
                void apply({
                  ...query,
                  page: 1,
                  search: "",
                  account: "all",
                  credential: "all",
                  health: "all",
                })
              }
            >
              清除筛选
            </WorkbenchButton>
          }
        />
      ) : (
        <div className="wb-table-wrap">
          <table className="wb-data-table" aria-label="用户目录">
            <thead>
              <tr>
                <th>用户</th>
                <th>账户状态</th>
                <th>平台连接</th>
                <th>设备 / 上限</th>
                <th>最近读取</th>
                <th>
                  <span className="wb-sr-only">操作</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {page.items.map((item) => {
                const user = item.user
                return (
                  <tr key={user.id}>
                    <td>
                      <button
                        className="wb-user-link"
                        onClick={() => updatePageQuery({ user: user.id })}
                      >
                        <span className="wb-avatar">
                          {user.username[0]?.toUpperCase()}
                        </span>
                        <span>
                          <strong>{user.username}</strong>
                          <small>
                            {user.id === currentUserId ? "当前账号 · " : ""}
                            {user.role === "admin" ? "管理员" : "普通用户"}
                          </small>
                        </span>
                      </button>
                    </td>
                    <td>
                      <StatusPill tone={user.enabled ? "idle" : "neutral"}>
                        {user.enabled ? "已启用" : "已停用"}
                      </StatusPill>
                    </td>
                    <td>
                      <StatusPill
                        tone={
                          user.role === "admin" ||
                          item.dashboard.pileCount === 0
                            ? "neutral"
                            : item.credential.state === "healthy"
                              ? "idle"
                              : ["expired", "sync_failed"].includes(
                                    item.credential.state
                                  )
                                ? "danger"
                                : "neutral"
                        }
                      >
                        {user.role === "admin"
                          ? "无需扫码"
                          : item.dashboard.pileCount === 0
                            ? "待添加设备"
                            : credentialLabels[item.credential.state]}
                      </StatusPill>
                    </td>
                    <td className="wb-mono">
                      {item.dashboard.pileCount} / {user.deviceLimit}
                    </td>
                    <td>
                      {formatSnapshotTime(item.lastRefresh.lastRemoteAt, true)}
                    </td>
                    <td>
                      <WorkbenchButton
                        variant="ghost"
                        aria-label={`管理用户 ${user.username}`}
                        onClick={() => updatePageQuery({ user: user.id })}
                      >
                        详情
                        <ChevronRightIcon size={14} />
                      </WorkbenchButton>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      {page && (
        <div className="wb-pagination">
          <span>
            共 {page.total} 个账户 · 第 {page.page} /{" "}
            {Math.max(1, page.totalPages)} 页
          </span>
          <div>
            <WorkbenchButton
              variant="ghost"
              disabled={page.page <= 1}
              onClick={() => void apply({ ...query, page: page.page - 1 })}
            >
              上一页
            </WorkbenchButton>
            <WorkbenchButton
              variant="ghost"
              disabled={page.page >= page.totalPages}
              onClick={() => void apply({ ...query, page: page.page + 1 })}
            >
              下一页
            </WorkbenchButton>
          </div>
        </div>
      )}
    </div>
  )
}

function WorkbenchAdminUserDetail({
  userId,
  currentUserId,
}: {
  userId: string
  currentUserId?: string
}) {
  const router = useRouter(),
    online = useOnline(),
    params = useSearchParams()
  const [detail, setDetail] = useState<AdminUserDetail | null>(null)
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)
  const [limit, setLimit] = useState("")
  const [role, setRole] = useState<UserRole>("user")
  const [modal, setModal] = useState<"password" | "delete" | "role" | null>(
    null
  )
  const [temporary, setTemporary] = useState("")
  const [confirmation, setConfirmation] = useState("")
  const tab = ["diagnostics", "devices"].includes(params.get("section") ?? "")
    ? params.get("section")
    : "account"
  const load = useCallback(async () => {
    try {
      const value = await adminApi.userDetail(userId)
      setDetail(value)
      setLimit(String(value.summary.user.deviceLimit))
      setRole(value.summary.user.role)
      setError("")
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "暂时无法读取用户详情。"
      )
    }
  }, [userId])
  useEffect(() => {
    let active = true
    adminApi
      .userDetail(userId)
      .then((value) => {
        if (active) {
          setDetail(value)
          setLimit(String(value.summary.user.deviceLimit))
          setRole(value.summary.user.role)
        }
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error ? reason.message : "暂时无法读取用户详情。"
          )
      })
    return () => {
      active = false
    }
  }, [userId])
  const user = detail?.summary.user,
    isCurrent = user?.id === currentUserId
  async function mutate(
    payload: Parameters<typeof adminApi.updateUser>[1],
    message: string
  ) {
    setPending(true)
    setError("")
    try {
      await adminApi.updateUser(userId, payload)
      await load()
      notify.success(message)
      setModal(null)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "更改未保存。")
    } finally {
      setPending(false)
    }
  }
  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (
      !Number.isInteger(Number(limit)) ||
      Number(limit) < 1 ||
      Number(limit) > 100
    ) {
      setError("设备上限需要是 1–100 之间的整数。")
      return
    }
    if (role !== user?.role) {
      setModal("role")
      return
    }
    await mutate({ deviceLimit: Number(limit) }, "设备上限已保存")
  }
  async function resetPassword() {
    setPending(true)
    try {
      const response = await adminApi.resetUserPassword(userId)
      setTemporary(response.temporaryPassword)
      await load()
      notify.success("临时密码已生成，原有登录会话已撤销")
    } catch (reason) {
      notify.error(reason, { title: "重置密码未完成" })
    } finally {
      setPending(false)
    }
  }
  async function remove() {
    setPending(true)
    try {
      await adminApi.removeUser(userId)
      notify.success("账户已删除，审计记录保留")
      router.push("/admin?tab=users")
    } catch (reason) {
      notify.error(reason, { title: "账户未删除" })
    } finally {
      setPending(false)
    }
  }
  async function refresh() {
    setPending(true)
    try {
      await adminApi.refreshUser(userId)
      await load()
      notify.success("该用户的设备状态已更新")
    } catch (reason) {
      notify.error(reason, { title: "刷新未成功，已保留最近记录" })
    } finally {
      setPending(false)
    }
  }
  if (!detail)
    return error ? (
      <WorkbenchError message={error} retry={() => void load()} />
    ) : (
      <WorkbenchLoading label="正在读取用户详情" />
    )
  const summary = detail.summary
  return (
    <>
      <button
        className="wb-back"
        onClick={() => updatePageQuery({ user: null, section: null })}
      >
        <ArrowLeftIcon size={15} />
        全部用户
      </button>
      <div className="wb-user-detail-title">
        <span className="wb-avatar wb-avatar-large">
          {user!.username[0]?.toUpperCase()}
        </span>
        <div>
          <h2>{user!.username}</h2>
          <p>
            {user!.role === "admin" ? "管理员" : "普通用户"}
            {isCurrent ? " · 当前账户" : ""}
          </p>
        </div>
        <StatusPill tone={user!.enabled ? "idle" : "neutral"}>
          {user!.enabled ? "已启用" : "已停用"}
        </StatusPill>
      </div>
      <div className="wb-section-tabs">
        {[
          ["account", "账户权限"],
          ["diagnostics", "连接诊断"],
          ["devices", "设备与会话"],
        ].map(([value, label]) => (
          <button
            key={value}
            aria-pressed={tab === value}
            onClick={() =>
              updatePageQuery({ section: value === "account" ? null : value })
            }
          >
            {label}
          </button>
        ))}
      </div>
      {error && (
        <p role="alert" className="wb-form-error">
          {error}
        </p>
      )}
      {tab === "account" ? (
        <div className="wb-settings-content">
          <div className="wb-setting-row">
            <div>
              <h3>启用账户</h3>
              <p>停用后不能继续登录，已有数据保留。</p>
            </div>
            <Switch
              aria-label="启用账户"
              checked={user!.enabled}
              disabled={!online || pending || isCurrent}
              onCheckedChange={(value) =>
                void mutate(
                  { enabled: value },
                  value ? "账户已启用" : "账户已停用"
                )
              }
            />
          </div>
          <div className="wb-setting-row">
            <div>
              <h3>允许刷新</h3>
              <p>控制该用户主动刷新和后台提醒的远端访问权限。</p>
            </div>
            <Switch
              aria-label="允许用户刷新"
              checked={user!.refreshEnabled}
              disabled={!online || pending || user!.role === "admin"}
              onCheckedChange={(value) =>
                void mutate({ refreshEnabled: value }, "刷新权限已保存")
              }
            />
          </div>
          <form className="wb-user-settings-form" onSubmit={save}>
            <Field>
              <FieldLabel htmlFor="user-role">账户角色</FieldLabel>
              <select
                id="user-role"
                className="wb-input"
                value={role}
                disabled={isCurrent || pending}
                onChange={(event) => setRole(event.target.value as UserRole)}
              >
                <option value="user">普通用户</option>
                <option value="admin">管理员</option>
              </select>
            </Field>
            <Field>
              <FieldLabel htmlFor="user-device-limit">设备上限</FieldLabel>
              <Input
                id="user-device-limit"
                type="number"
                min={1}
                max={100}
                value={limit}
                onChange={(event) => setLimit(event.target.value)}
                disabled={pending}
              />
              <p className="wb-small wb-muted">
                当前已添加 {summary.dashboard.pileCount}{" "}
                台，降低上限不会删除设备。
              </p>
            </Field>
            <WorkbenchButton type="submit" busy={pending} disabled={!online}>
              保存更改
            </WorkbenchButton>
          </form>
          <div className="wb-setting-row">
            <div>
              <h3>重置登录密码</h3>
              <p>生成一次性临时密码，并退出该用户的所有设备。</p>
            </div>
            <WorkbenchButton
              variant="outline"
              disabled={!online || pending || isCurrent}
              onClick={() => {
                setTemporary("")
                setModal("password")
              }}
            >
              <KeyRoundIcon size={14} />
              重置密码
            </WorkbenchButton>
          </div>
          <div className="wb-setting-row">
            <div>
              <h3>删除账户</h3>
              <p>删除账户、设备、凭据和会话；审计日志保留。</p>
            </div>
            <WorkbenchButton
              variant="destructive"
              disabled={!online || pending || isCurrent}
              onClick={() => {
                setConfirmation("")
                setModal("delete")
              }}
            >
              <Trash2Icon size={14} />
              删除账户
            </WorkbenchButton>
          </div>
        </div>
      ) : tab === "diagnostics" ? (
        <div className="wb-settings-content">
          <SectionHeading title="平台连接" />
          <div className="wb-setting-row">
            <div>
              <h3>
                {user!.role === "admin"
                  ? "管理员不需要绑定充电平台"
                  : credentialLabels[summary.credential.state]}
              </h3>
              <p>
                {summary.credential.state === "expired"
                  ? "需要用户本人重新扫码，管理员不能代替其授权。"
                  : "这里只展示连接摘要，不会显示 Cookie、OpenID 或其他访问凭据。"}
              </p>
            </div>
            <StatusPill
              tone={summary.credential.state === "healthy" ? "idle" : "neutral"}
            >
              {user!.role === "admin"
                ? "不适用"
                : credentialLabels[summary.credential.state]}
            </StatusPill>
          </div>
          <AdminUserRefreshTiming
            bound={summary.credential.bound}
            lastCheckedAt={summary.credential.lastCheckedAt}
            lastRemoteAt={summary.lastRefresh.lastRemoteAt}
          />
          <dl className="wb-diagnostic-facts">
            {[
              ["最近快照", formatSnapshotTime(summary.snapshotUpdatedAt, true)],
              [
                "下次可刷新",
                formatSnapshotTime(summary.lastRefresh.nextRemoteAt, true),
              ],
              [
                "下次重试",
                formatSnapshotTime(summary.lastRefresh.nextRetryAt, true),
              ],
              [
                "请求 / 远端读取",
                `${summary.stats.totalRequests} / ${summary.stats.remoteFetches}`,
              ],
              [
                "失败 / 登录异常",
                `${summary.stats.failedRequests} / ${summary.stats.authFailures}`,
              ],
              ["缓存复用", `${summary.stats.cachedRefreshes} 次`],
            ].map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>{value}</dd>
              </div>
            ))}
          </dl>
          <WorkbenchButton
            variant="outline"
            busy={pending}
            disabled={!online || user!.role === "admin"}
            onClick={() => void refresh()}
          >
            <RefreshCwIcon size={15} />
            为该用户刷新一次
          </WorkbenchButton>
          <SectionHeading title="最近恢复记录" />
          <AdminUserDiagnostics
            diagnostics={summary.recoveryDiagnostics ?? []}
          />
        </div>
      ) : (
        <div className="wb-settings-content">
          <SectionHeading title={`已绑定设备 · ${detail.piles.length}`} />
          {detail.piles.length ? (
            detail.piles.map((pile) => (
              <div className="wb-admin-device-row" key={pile.id}>
                <span className="wb-mono">
                  {pile.number || pile.id.slice(-8)}
                </span>
                <div>
                  <strong>{pile.name || pile.number}</strong>
                  <p className="wb-small wb-muted">
                    {pile.address || "未填写位置"}
                  </p>
                </div>
                <StatusPill tone={pile.online ? "neutral" : "offline"}>
                  {pile.ports.length
                    ? pile.online
                      ? `${pile.ports.length} 个口`
                      : "离线"
                    : "尚未读取"}
                </StatusPill>
              </div>
            ))
          ) : (
            <p className="wb-muted">还没有绑定设备。</p>
          )}
          <SectionHeading title={`登录会话 · ${detail.sessions.length}`} />
          {detail.sessions.length ? (
            detail.sessions.map((session) => (
              <div className="wb-session-row" key={session.id}>
                <div>
                  <h3>
                    {session.browser} · {session.os}
                  </h3>
                  <p>
                    {session.ipLabel} · 最近活跃{" "}
                    {formatSnapshotTime(session.lastActiveAt, true)} · 到期{" "}
                    {formatSnapshotTime(session.expiresAt, true)}
                  </p>
                </div>
              </div>
            ))
          ) : (
            <p className="wb-muted">当前没有活动会话。</p>
          )}
        </div>
      )}
      <Dialog
        open={modal !== null}
        onOpenChange={(open) => {
          if (!open && !pending) {
            setModal(null)
            setTemporary("")
          }
        }}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>
              {modal === "password"
                ? temporary
                  ? "临时密码已生成"
                  : "重置用户密码"
                : modal === "role"
                  ? "确认变更账户角色？"
                  : `删除账户 ${user!.username}？`}
            </DialogTitle>
            <DialogDescription>
              {modal === "password"
                ? temporary
                  ? "临时密码只显示这一次，请通过安全渠道交给本人，下次登录后需修改。"
                  : "会撤销全部登录会话，并生成随机临时密码。"
                : modal === "role"
                  ? `此账户将变为${role === "admin" ? "管理员，可管理其他用户和系统策略" : "普通用户，不再拥有管理权限"}。`
                  : "账户和关联数据无法恢复，请输入完整用户名确认。"}
            </DialogDescription>
          </DialogHeader>
          {modal === "delete" && (
            <Field>
              <FieldLabel htmlFor="delete-username">要删除的用户名</FieldLabel>
              <Input
                id="delete-username"
                value={confirmation}
                autoComplete="off"
                onChange={(event) => setConfirmation(event.target.value)}
              />
            </Field>
          )}
          {temporary && (
            <div className="wb-temporary-password">
              <code className="break-all select-all">{temporary}</code>
              <WorkbenchButton
                variant="ghost"
                aria-label="复制临时密码"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(temporary)
                    notify.success("临时密码已复制")
                  } catch {
                    notify.error("请手动选择并复制临时密码。")
                  }
                }}
              >
                <CopyIcon size={15} />
              </WorkbenchButton>
            </div>
          )}
          <DialogFooter>
            <WorkbenchButton
              variant="outline"
              disabled={pending}
              onClick={() => {
                setModal(null)
                setTemporary("")
              }}
            >
              {temporary ? "我已保存" : "取消"}
            </WorkbenchButton>
            {!temporary && (
              <WorkbenchButton
                variant={modal === "delete" ? "destructive" : "default"}
                busy={pending}
                disabled={
                  !online ||
                  (modal === "delete" && confirmation !== user!.username)
                }
                onClick={() =>
                  void (modal === "password"
                    ? resetPassword()
                    : modal === "role"
                      ? mutate(
                          { role, deviceLimit: Number(limit) },
                          "账户权限已更新"
                        )
                      : remove())
                }
              >
                {modal === "password"
                  ? "生成临时密码"
                  : modal === "role"
                    ? "确认变更"
                    : "确认删除"}
              </WorkbenchButton>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
