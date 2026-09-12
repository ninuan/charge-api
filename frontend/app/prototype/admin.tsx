"use client"

import { useState, type FormEvent } from "react"
import {
  ArrowRight,
  Check,
  CheckCheck,
  ChevronRight,
  ClipboardList,
  Copy,
  Database,
  Download,
  HardDrive,
  KeyRound,
  Plus,
  RefreshCw,
  Server,
  ShieldAlert,
  Trash2,
  Users,
} from "lucide-react"
import type { RegistrationSettings } from "@/lib/types"
import { usePrototype } from "./context"
import { credentialLabel, type DemoIncident, type DemoUser } from "./model"
import {
  BackLink,
  Badge,
  EmptyState,
  ErrorState,
  Field,
  FormError,
  LoadingState,
  PageHeading,
  PButton,
  PInput,
  SaveButton,
  SearchField,
  SectionHeading,
  SettingRow,
  Toggle,
} from "./ui"

export function AdminPage() {
  const { route, scenario } = usePrototype()
  if (scenario === "loading")
    return (
      <div className="cp-standard-page">
        <LoadingState label="正在读取管理数据" />
      </div>
    )
  if (scenario === "error")
    return (
      <div className="cp-standard-page">
        <ErrorState />
      </div>
    )
  if (route.view === "users") return <UsersPage />
  if (route.view === "incidents") return <IncidentsPage />
  if (route.view === "operations") return <OperationsPage />
  if (route.view === "policies") return <PoliciesPage />
  if (route.view === "invites") return <InvitesPage />
  if (route.view === "audit") return <AuditPage />
  return <AdminOverview />
}

function AdminOverview() {
  const { data, go, notify, scenario } = usePrototype()
  const [range, setRange] = useState("24h")
  const open = data.incidents.filter((i) => i.status !== "resolved")
  const series =
    range === "24h"
      ? [3, 2, 1, 1, 4, 8, 14, 12, 7, 9, 8, 11]
      : range === "7d"
        ? [12, 18, 14, 21, 17, 24, 19]
        : [
            8, 9, 12, 6, 8, 17, 12, 8, 21, 19, 15, 12, 9, 12, 8, 17, 18, 20, 19,
            11, 14, 13, 9, 7, 18, 23, 17, 20, 16, 19,
          ]
  function exportCSV() {
    const csv =
      "\ufeff时段,远端请求,失败请求\n" +
      series.map((n, i) => `${i + 1},${n},${i === 4 ? 1 : 0}`).join("\n")
    const url = URL.createObjectURL(
      new Blob([csv], { type: "text/csv;charset=utf-8" })
    )
    const link = document.createElement("a")
    link.href = url
    link.download = `charge-demo-trends-${range}.csv`
    link.click()
    URL.revokeObjectURL(url)
    notify("已导出当前范围的示例趋势 CSV。")
  }
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="运行概览"
        description="先处理需要关注的事，再了解系统运行情况。"
        actions={
          <PButton onClick={() => notify("管理状态已更新（示例）。")}>
            <RefreshCw size={15} />
            更新状态
          </PButton>
        }
      />
      <div className="cp-admin-healthline">
        <Badge tone="idle">主服务正常</Badge>
        <span>数据库正常</span>
        <span>扫码服务正常</span>
        <button
          className="cp-text-link"
          onClick={() => go({ view: "operations" })}
        >
          查看系统运行
          <ArrowRight size={14} />
        </button>
      </div>
      <section className="cp-admin-priority">
        <SectionHeading
          title={`需要关注 · ${scenario === "empty" ? 0 : open.length}`}
          description="按对用户的影响排序。"
        >
          <PButton tone="quiet" onClick={() => go({ view: "incidents" })}>
            全部异常
            <ArrowRight size={14} />
          </PButton>
        </SectionHeading>
        {scenario === "empty" || !open.length ? (
          <EmptyState
            icon={CheckCheck}
            title="目前没有待处理异常"
            description="有新的连接或投递问题时，会集中显示在这里。"
          />
        ) : (
          open.map((issue) => (
            <button
              key={issue.id}
              className="cp-priority-row"
              onClick={() => go({ view: "incidents", id: issue.id })}
            >
              <span className={`cp-incident-dot cp-${issue.level}`} />
              <span>
                <strong>{issue.title}</strong>
                <small>
                  {data.users.find((u) => u.id === issue.userId)?.username} ·
                  已发生 {issue.occurrences} 次
                </small>
              </span>
              <Badge
                tone={issue.status === "open" ? "warning" : "neutral"}
                dot={false}
              >
                {issue.status === "open" ? "待处理" : "跟进中"}
              </Badge>
              <time>{issue.time}</time>
              <ChevronRight size={15} />
            </button>
          ))
        )}
      </section>
      <section className="cp-admin-trends">
        <SectionHeading
          title="请求与使用"
          description="区分真实远端请求与缓存命中，了解服务负载。"
        >
          <div className="cp-row-actions">
            <div className="cp-segmented">
              {[
                ["24h", "24 小时"],
                ["7d", "7 天"],
                ["30d", "30 天"],
              ].map(([v, label]) => (
                <button
                  key={v}
                  aria-pressed={range === v}
                  onClick={() => setRange(v)}
                >
                  {label}
                </button>
              ))}
            </div>
            <PButton tone="quiet" onClick={exportCSV}>
              <Download size={14} />
              导出 CSV
            </PButton>
          </div>
        </SectionHeading>
        <div className="cp-admin-inline-metrics">
          <span>
            <strong>
              {data.users.filter((u) => u.enabled && u.role === "user").length}
            </strong>{" "}
            活跃账户
          </span>
          <span>
            <strong>{data.users.reduce((s, u) => s + u.deviceCount, 0)}</strong>{" "}
            受管设备
          </span>
          <span>
            <strong>{series.reduce((s, n) => s + n, 0)}</strong> 远端请求
          </span>
          <span>
            <strong>
              {((1 - 1 / series.reduce((s, n) => s + n, 0)) * 100).toFixed(1)}
              <small>%</small>
            </strong>{" "}
            请求成功
          </span>
        </div>
        <div className="cp-admin-bar-chart" aria-label="远端请求趋势示例">
          {series.map((n, i) => (
            <button
              key={i}
              title={`${i + 1} 时段：${n} 次远端请求`}
              aria-label={`${i + 1} 时段，${n} 次远端请求`}
              onClick={() =>
                notify(
                  `第 ${i + 1} 时段：${n} 次远端请求，${i === 4 ? 1 : 0} 次失败。`,
                  "info"
                )
              }
            >
              <i style={{ height: `${(n / 26) * 100}%` }} />
              <span>
                {series.length <= 12 || i % 7 === 0
                  ? range === "24h"
                    ? `${i * 2}:00`
                    : `${i + 1}`
                  : ""}
              </span>
            </button>
          ))}
        </div>
        <div className="cp-chart-caption">
          <span>
            <i className="cp-legend-busy" />
            远端请求 / 次
          </span>
          <span>示例数据 · 亚洲 / 上海</span>
        </div>
      </section>
    </div>
  )
}

function UsersPage() {
  const { data, route, go, openModal, scenario } = usePrototype()
  const [query, setQuery] = useState("")
  const [accountFilter, setAccountFilter] = useState("all")
  const [credentialFilter, setCredentialFilter] = useState("all")
  const selected = data.users.find((u) => u.id === route.id)
  const items = (scenario === "empty" ? [] : data.users)
    .filter((u) => u.username.toLowerCase().includes(query.toLowerCase()))
    .filter(
      (u) =>
        accountFilter === "all" ||
        (accountFilter === "enabled" ? u.enabled : !u.enabled)
    )
    .filter(
      (u) =>
        credentialFilter === "all" ||
        (credentialFilter === "risk"
          ? ["expired", "sync_failed"].includes(u.credential)
          : u.credential === credentialFilter)
    )
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="用户"
        description="账户访问、设备额度与连接问题，在同一处处理。"
        actions={
          <PButton
            tone="brand"
            onClick={() => openModal({ kind: "create-user" })}
          >
            <Plus size={15} />
            创建用户
          </PButton>
        }
      />
      {selected ? (
        <UserDetail key={selected.id} user={selected} />
      ) : (
        <>
          <div className="cp-page-rule">
            <SearchField
              label="搜索用户"
              placeholder="搜索用户名"
              value={query}
              onChange={setQuery}
            />
            <div className="cp-table-filters">
              <label className="cp-inline-select">
                <span className="cp-sr-only">账户状态</span>
                <select
                  aria-label="账户状态"
                  value={accountFilter}
                  onChange={(e) => setAccountFilter(e.target.value)}
                >
                  <option value="all">全部账户</option>
                  <option value="enabled">已启用</option>
                  <option value="disabled">已停用</option>
                </select>
              </label>
              <label className="cp-inline-select">
                <span className="cp-sr-only">连接状态</span>
                <select
                  aria-label="连接状态"
                  value={credentialFilter}
                  onChange={(e) => setCredentialFilter(e.target.value)}
                >
                  <option value="all">全部连接状态</option>
                  <option value="risk">需要关注</option>
                  {Object.entries(credentialLabel).map(([v, label]) => (
                    <option key={v} value={v}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          </div>
          {!items.length ? (
            <EmptyState
              icon={Users}
              title="没有匹配的用户"
              description="调整搜索或筛选条件，或者创建一个新账户。"
              action={
                <PButton
                  onClick={() => {
                    setQuery("")
                    setAccountFilter("all")
                    setCredentialFilter("all")
                  }}
                >
                  清除筛选
                </PButton>
              }
            />
          ) : (
            <div className="cp-table-wrap">
              <table className="cp-data-table">
                <thead>
                  <tr>
                    <th>用户</th>
                    <th>账户状态</th>
                    <th>平台连接</th>
                    <th>设备 / 上限</th>
                    <th>最近活跃</th>
                    <th>
                      <span className="cp-sr-only">操作</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((u) => (
                    <tr key={u.id}>
                      <td>
                        <button
                          className="cp-user-link"
                          onClick={() => go({ view: "users", id: u.id })}
                        >
                          <span className="cp-avatar">
                            {u.username.slice(0, 1).toUpperCase()}
                          </span>
                          <span>
                            <strong>{u.username}</strong>
                            <small>
                              {u.role === "admin" ? "管理员" : "普通用户"}
                            </small>
                          </span>
                        </button>
                      </td>
                      <td>
                        <Badge tone={u.enabled ? "idle" : "neutral"}>
                          {u.enabled ? "已启用" : "已停用"}
                        </Badge>
                      </td>
                      <td>
                        <Badge
                          tone={
                            u.credential === "healthy"
                              ? "idle"
                              : ["expired", "sync_failed"].includes(
                                    u.credential
                                  )
                                ? "danger"
                                : "neutral"
                          }
                          dot={false}
                        >
                          {u.role === "admin"
                            ? "不适用"
                            : credentialLabel[u.credential]}
                        </Badge>
                      </td>
                      <td className="cp-mono">
                        {u.deviceCount} / {u.deviceLimit}
                      </td>
                      <td>{u.lastActive}</td>
                      <td>
                        <PButton
                          tone="quiet"
                          onClick={() => go({ view: "users", id: u.id })}
                        >
                          详情
                          <ChevronRight size={14} />
                        </PButton>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="cp-table-caption">
            共 {items.length} 个账户 · 设备和凭据按用户独立保存
          </p>
        </>
      )}
    </div>
  )
}

function UserDetail({ user }: { user: DemoUser }) {
  const { data, go, openModal, commit, addAudit, offline, busy } =
    usePrototype()
  const [limit, setLimit] = useState(String(user.deviceLimit))
  const [role, setRole] = useState(user.role)
  const [error, setError] = useState("")
  const [tab, setTab] = useState("account")
  async function save(e: FormEvent) {
    e.preventDefault()
    if (
      !Number.isInteger(Number(limit)) ||
      Number(limit) < 0 ||
      Number(limit) > 100
    ) {
      setError("设备上限必须是 0–100 之间的整数。")
      return
    }
    setError("")
    await commit((d) => {
      const target = d.users.find((u) => u.id === user.id)
      if (target) {
        target.deviceLimit = Number(limit)
        target.role = role
      }
      addAudit(d, "更新用户权限与额度", user.username)
    }, "用户设置已保存。")
  }
  return (
    <>
      <BackLink onClick={() => go({ view: "users" })}>全部用户</BackLink>
      <div className="cp-user-detail-title">
        <span className="cp-avatar cp-avatar-large">
          {user.username.slice(0, 1).toUpperCase()}
        </span>
        <div>
          <h2>{user.username}</h2>
          <p>
            {user.id} · {user.role === "admin" ? "管理员" : "普通用户"}
          </p>
        </div>
        <Badge tone={user.enabled ? "idle" : "neutral"}>
          {user.enabled ? "已启用" : "已停用"}
        </Badge>
      </div>
      <div className="cp-section-tabs">
        {[
          ["account", "账户权限"],
          ["diagnostics", "连接诊断"],
          ["devices", "设备与会话"],
        ].map(([v, label]) => (
          <button key={v} aria-pressed={tab === v} onClick={() => setTab(v)}>
            {label}
          </button>
        ))}
      </div>
      {tab === "account" ? (
        <div className="cp-settings-content">
          <SettingRow
            title="启用账户"
            description="停用后将无法登录，不会删除已有数据。"
          >
            <Toggle
              label="启用账户"
              checked={user.enabled}
              disabled={user.id === "admin-1" || offline}
              onChange={(v) =>
                void commit(
                  (d) => {
                    const target = d.users.find((u) => u.id === user.id)
                    if (target) {
                      target.enabled = v
                      if (!v) target.sessions = 0
                    }
                    addAudit(d, v ? "启用用户" : "停用用户", user.username)
                  },
                  v ? "账户已启用。" : "账户已停用，已有会话已退出。"
                )
              }
            />
          </SettingRow>
          <SettingRow
            title="允许刷新"
            description="关闭后，该用户不能触发远端刷新或后台提醒。"
          >
            <Toggle
              label="允许用户刷新"
              checked={user.refreshEnabled}
              disabled={offline || user.role === "admin"}
              onChange={(v) =>
                void commit((d) => {
                  const target = d.users.find((u) => u.id === user.id)
                  if (target) target.refreshEnabled = v
                  addAudit(d, "调整刷新权限", user.username)
                }, "刷新权限已保存。")
              }
            />
          </SettingRow>
          <form className="cp-user-settings-form" onSubmit={save}>
            <Field label="账户角色">
              <select
                className="cp-input"
                value={role}
                disabled={user.id === "admin-1"}
                onChange={(e) => setRole(e.target.value as "admin" | "user")}
              >
                <option value="user">普通用户</option>
                <option value="admin">管理员</option>
              </select>
            </Field>
            <Field
              label="设备上限"
              hint={`当前已添加 ${user.deviceCount} 台；降低上限不会删除现有设备。`}
            >
              <PInput
                type="number"
                min={0}
                max={100}
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
              />
            </Field>
            <FormError message={error} />
            <SaveButton />
          </form>
          <SettingRow
            title="重置登录密码"
            description="生成一次性临时密码，并撤销该用户的全部会话。"
          >
            <PButton
              disabled={offline || user.id === "admin-1"}
              onClick={() => openModal({ kind: "reset-password", id: user.id })}
            >
              <KeyRound size={15} />
              重置密码
            </PButton>
          </SettingRow>
          <SettingRow
            title="删除账户"
            description="将移除账户及关联数据；请确认不再需要保留。"
          >
            <PButton
              tone="danger"
              disabled={user.id === "admin-1" || offline}
              onClick={() => openModal({ kind: "delete-user", id: user.id })}
            >
              <Trash2 size={14} />
              删除账户
            </PButton>
          </SettingRow>
        </div>
      ) : tab === "diagnostics" ? (
        <div className="cp-settings-content">
          <SectionHeading title="平台连接" />
          <SettingRow
            title={
              user.role === "admin"
                ? "管理员不绑定充电平台"
                : credentialLabel[user.credential]
            }
            description={
              user.credential === "expired"
                ? "自动恢复失败，需要由用户本人重新扫码；管理员无法代替用户授权。"
                : "这里只展示连接摘要，不展示 Cookie、OpenID 或其他敏感凭据。"
            }
          >
            <Badge tone={user.credential === "healthy" ? "idle" : "warning"}>
              {user.role === "admin"
                ? "不适用"
                : credentialLabel[user.credential]}
            </Badge>
          </SettingRow>
          <dl className="cp-diagnostic-facts">
            <div>
              <dt>最近一次刷新</dt>
              <dd>今天 {user.lastActive}</dd>
            </div>
            <div>
              <dt>下次可重试</dt>
              <dd>{user.credential === "expired" ? "重新登录后" : "现在"}</dd>
            </div>
            <div>
              <dt>今日请求</dt>
              <dd>{user.requests} 次</dd>
            </div>
            <div>
              <dt>数据来源</dt>
              <dd>最近一次保存的快照</dd>
            </div>
          </dl>
          <PButton
            disabled={offline || user.role === "admin"}
            loading={!!busy}
            onClick={() => {
              if (user.credential === "expired") {
                void commit(
                  (d) => {
                    addAudit(d, "刷新失败：登录已失效", user.username)
                    d.audit[0].result = "failure"
                  },
                  "示例刷新未成功，需要用户重新扫码。",
                  "error"
                )
                return
              }
              void commit((d) => {
                const target = d.users.find((u) => u.id === user.id)
                if (target) target.requests++
                addAudit(d, "手动刷新用户设备", user.username)
              }, "已刷新该用户的示例设备状态。")
            }}
          >
            <RefreshCw size={14} />
            为该用户刷新一次
          </PButton>
          <SectionHeading title="近期恢复记录" />
          <div className="cp-log-line">
            <span className="cp-mono">18:35:12</span>
            <span>
              {user.credential === "expired"
                ? "自动恢复失败 · 需要重新扫码"
                : "凭据检查正常"}
            </span>
          </div>
          {data.incidents
            .filter((i) => i.userId === user.id)
            .map((i) => (
              <button
                key={i.id}
                className="cp-text-link"
                onClick={() => go({ view: "incidents", id: i.id })}
              >
                查看关联异常：{i.title}
                <ArrowRight size={14} />
              </button>
            ))}
        </div>
      ) : (
        <div className="cp-settings-content">
          <SectionHeading
            title={`已绑定设备 · ${user.deviceCount}`}
            description="管理员只查看设备摘要，不会进入用户的个人工作区。"
          />
          {user.deviceCount === 0 ? (
            <p className="cp-muted">该账户还没有绑定设备。</p>
          ) : (
            data.piles.slice(0, user.deviceCount).map((p, i) => (
              <div className="cp-admin-device-row" key={p.id}>
                <span className="cp-mono">
                  {user.id === "user-1" ? p.number : `70${i + 1}20${i + 1}`}
                </span>
                <span>
                  {user.id === "user-1" ? p.name : `常用充电桩 ${i + 1}`}
                </span>
                <Badge tone={p.online ? "idle" : "offline"}>
                  {p.online ? "已连接" : "离线"}
                </Badge>
              </div>
            ))
          )}
          <SectionHeading title={`登录会话 · ${user.sessions}`} />
          {user.sessions ? (
            <>
              <div className="cp-log-line">
                <span>Chrome · macOS</span>
                <span>最近活跃：{user.lastActive} · 本地网络</span>
              </div>
              {user.sessions > 1 && (
                <div className="cp-log-line">
                  <span>Safari · iOS</span>
                  <span>今天 12:38 · 移动网络</span>
                </div>
              )}
            </>
          ) : (
            <p className="cp-muted">没有活动会话。</p>
          )}
        </div>
      )}
    </>
  )
}

function IncidentsPage() {
  const { data, route, go, scenario } = usePrototype()
  const [filter, setFilter] = useState("active")
  const [query, setQuery] = useState("")
  const issue = data.incidents.find((i) => i.id === route.id)
  const items = (scenario === "empty" ? [] : data.incidents)
    .filter(
      (i) =>
        filter === "all" ||
        (filter === "active" ? i.status !== "resolved" : i.status === filter)
    )
    .filter((i) =>
      `${i.title} ${data.users.find((u) => u.id === i.userId)?.username}`.includes(
        query.trim()
      )
    )
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="异常处理"
        description="定位影响、记录处理过程，再关闭问题。"
      />
      {issue ? (
        <IncidentDetail key={issue.id} issue={issue} />
      ) : (
        <>
          <div className="cp-page-rule">
            <div className="cp-section-tabs">
              {[
                ["active", "待处理"],
                ["acknowledged", "跟进中"],
                ["resolved", "已解决"],
                ["all", "全部"],
              ].map(([v, label]) => (
                <button
                  key={v}
                  aria-pressed={filter === v}
                  onClick={() => setFilter(v)}
                >
                  {label}
                </button>
              ))}
            </div>
            <SearchField
              label="搜索异常"
              placeholder="搜索异常或用户名"
              value={query}
              onChange={setQuery}
            />
          </div>
          {!items.length ? (
            <EmptyState
              icon={CheckCheck}
              title="没有匹配的异常"
              description="系统出现需要关注的问题时，会在这里记录。"
            />
          ) : (
            items.map((i) => (
              <button
                className="cp-priority-row"
                key={i.id}
                onClick={() => go({ view: "incidents", id: i.id })}
              >
                <span className={`cp-incident-dot cp-${i.level}`} />
                <span>
                  <strong>{i.title}</strong>
                  <small>
                    {data.users.find((u) => u.id === i.userId)?.username} ·{" "}
                    {i.occurrences} 次 · 最近 {i.time}
                  </small>
                </span>
                <Badge
                  tone={
                    i.status === "resolved"
                      ? "idle"
                      : i.status === "acknowledged"
                        ? "neutral"
                        : "warning"
                  }
                  dot={false}
                >
                  {i.status === "resolved"
                    ? "已解决"
                    : i.status === "acknowledged"
                      ? "跟进中"
                      : "待处理"}
                </Badge>
                <ChevronRight size={15} />
              </button>
            ))
          )}
        </>
      )}
    </div>
  )
}
function IncidentDetail({ issue }: { issue: DemoIncident }) {
  const { data, go, commit, addAudit, offline, busy } = usePrototype()
  const [note, setNote] = useState(issue.note)
  const [error, setError] = useState("")
  async function update(status: DemoIncident["status"]) {
    if (status === "resolved" && note.trim().length < 2) {
      setError("请简要记录处理结果，再标记为解决。")
      return
    }
    setError("")
    await commit(
      (d) => {
        const target = d.incidents.find((i) => i.id === issue.id)
        if (target) {
          target.note = note
          target.status = status
        }
        addAudit(
          d,
          status === "resolved"
            ? "解决异常"
            : status === "open"
              ? "重新打开异常"
              : "确认异常",
          issue.title
        )
      },
      status === "resolved"
        ? "已记录处理结果，并标记为解决。"
        : "异常处理记录已更新。"
    )
  }
  return (
    <section className="cp-incident-detail">
      <BackLink onClick={() => go({ view: "incidents" })}>全部异常</BackLink>
      <div className="cp-incident-title">
        <ShieldAlert size={25} />
        <h2>{issue.title}</h2>
        <Badge tone={issue.status === "resolved" ? "idle" : "warning"}>
          {issue.status === "resolved"
            ? "已解决"
            : issue.status === "acknowledged"
              ? "跟进中"
              : "待处理"}
        </Badge>
      </div>
      <p>{issue.detail}</p>
      <dl className="cp-inline-facts">
        <div>
          <dt>受影响账户</dt>
          <dd>
            <button
              className="cp-text-link"
              onClick={() => go({ view: "users", id: issue.userId })}
            >
              {data.users.find((u) => u.id === issue.userId)?.username ||
                "已删除账户"}
              <ArrowRight size={14} />
            </button>
          </dd>
        </div>
        <div>
          <dt>累计发生</dt>
          <dd>{issue.occurrences} 次</dd>
        </div>
        <div>
          <dt>最近发生</dt>
          <dd>今天 {issue.time}</dd>
        </div>
      </dl>
      <Field label="处理记录" hint="记录你做了什么、还有什么需要跟进。">
        <textarea
          className="cp-input cp-textarea"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="例如：已联系用户重新扫码，确认刷新恢复。"
          maxLength={500}
        />
      </Field>
      <FormError message={error} />
      <div className="cp-row-actions">
        {issue.status === "resolved" ? (
          <PButton
            disabled={offline || !!busy}
            onClick={() => void update("open")}
          >
            重新打开
          </PButton>
        ) : (
          <>
            <PButton
              tone="brand"
              disabled={offline || !!busy}
              onClick={() => void update("resolved")}
            >
              <Check size={15} />
              标记为解决
            </PButton>
            <PButton
              disabled={offline || !!busy}
              onClick={() => void update("acknowledged")}
            >
              记录并跟进
            </PButton>
          </>
        )}
        <PButton
          tone="quiet"
          onClick={() => go({ view: "users", id: issue.userId })}
        >
          查看用户诊断
          <ArrowRight size={14} />
        </PButton>
      </div>
    </section>
  )
}

function OperationsPage() {
  const { data, notify, go, scenario } = usePrototype()
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="系统运行"
        description="服务、提醒调度、消息投递和数据保存情况。"
        actions={
          <PButton onClick={() => notify("系统运行状态已重新读取（示例）。")}>
            <RefreshCw size={15} />
            重新检查
          </PButton>
        }
      />
      <section className="cp-operations-section">
        <SectionHeading title="服务健康" />
        {[
          ["Charge 主服务", "API 与事件流正常", Server],
          ["SQLite 数据库", "完整性检查通过 · 12.6 MB", Database],
          ["扫码登录服务", "已配置 · 最近检查正常", KeyRound],
        ].map(([title, description, Icon]) => {
          const ServiceIcon = Icon as typeof Server
          return (
            <div className="cp-operation-row" key={String(title)}>
              <ServiceIcon size={20} />
              <div>
                <strong>{String(title)}</strong>
                <p>{String(description)}</p>
              </div>
              <Badge tone="idle">正常</Badge>
            </div>
          )
        })}
      </section>
      <section className="cp-operations-section">
        <SectionHeading title="后台提醒" />
        <SettingRow
          title={
            scenario === "power-off"
              ? "计划断电，调度已暂停"
              : data.settings.backgroundRemindersEnabled
                ? "调度器正在运行"
                : "后台提醒已关闭"
          }
          description={`当前 ${data.watches.filter((w) => w.status === "active").length} 条临时提醒，检查间隔 ${data.settings.watchRefreshIntervalMinutes} 分钟。`}
        >
          <PButton tone="quiet" onClick={() => go({ view: "policies" })}>
            调整策略
            <ArrowRight size={14} />
          </PButton>
        </SettingRow>
        <dl className="cp-ops-facts">
          {[
            ["等待检查", "1 台"],
            ["正在检查", "0 台"],
            ["24 小时远端请求", "18 次"],
            ["请求成功", "18 / 18"],
            ["缓存复用", "6 次"],
            ["合并请求", "2 次"],
            ["额度跳过", scenario === "quota" ? "1 次" : "0 次"],
            ["调度错误", "0 次"],
            ["已通知结束", "1 条"],
            ["已到期结束", "1 条"],
            ["平均等待", "42 分钟"],
            ["下次检查", "18:45"],
          ].map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </section>
      <section className="cp-operations-section">
        <SectionHeading
          title="微信消息投递"
          description="投递提交与手机送达是不同阶段，不混用成功率。"
        />
        <SettingRow
          title="投递服务运行中"
          description="1 条消息等待重试，站内通知不受影响。"
        >
          <Badge tone="warning">需要关注</Badge>
        </SettingRow>
        <dl className="cp-ops-facts">
          {[
            ["已绑定账户", "3"],
            ["待发送", "0"],
            ["等待重试", "1"],
            ["结果待确认", "0"],
            ["24 小时提交", "12 / 13"],
            ["供应商确认处理", "11 / 12"],
            ["绑定异常", "0"],
            ["系统失败", "1"],
          ].map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </section>
      <section className="cp-operations-section">
        <SectionHeading title="数据与备份" />
        <div className="cp-operation-row">
          <HardDrive size={21} />
          <div>
            <strong>最近备份 · 今天 03:00</strong>
            <p>12.1 MB · 备份任务在服务器上执行</p>
          </div>
          <Badge tone="idle">备份正常</Badge>
        </div>
        <dl className="cp-ops-facts">
          {[
            ["统计记录", "2,416 条"],
            ["端口历史", "18,204 条"],
            ["历史范围", "8 月 12 日至今"],
            ["通知记录", `${data.notices.length} 条`],
            ["统计保留", `${data.settings.statsRetentionDays} 天`],
            ["端口历史保留", `${data.settings.portHistoryRetentionDays} 天`],
            ["通知保留", `${data.settings.notificationRetentionDays} 天`],
            ["完整性检查", "通过"],
          ].map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
        <p className="cp-data-note">
          仍待处理的通知不会随普通保留策略被清理。此页面不执行服务器备份或数据库删除。
        </p>
      </section>
    </div>
  )
}

function PoliciesPage() {
  const { data, commit, addAudit } = usePrototype()
  const [draft, setDraft] = useState<RegistrationSettings>(data.settings)
  const [error, setError] = useState("")
  const changed = JSON.stringify(draft) !== JSON.stringify(data.settings)
  function number(key: keyof RegistrationSettings, value: string) {
    setDraft((d) => ({ ...d, [key]: value === "" ? 0 : Number(value) }))
  }
  function toggle(key: keyof RegistrationSettings, value: boolean) {
    setDraft((d) => ({ ...d, [key]: value }))
  }
  async function save(e: FormEvent) {
    e.preventDefault()
    const fields = [
      draft.defaultDeviceLimit,
      draft.statsRetentionDays,
      draft.portHistoryRetentionDays,
      draft.watchRefreshIntervalMinutes,
      draft.watchPileLimitPerUser,
      draft.watchDailyRefreshQuota,
      draft.notificationRetentionDays,
    ]
    if (fields.some((n) => !Number.isInteger(n) || n < 1 || n > 10000)) {
      setError("上限、间隔和保留天数需要填写大于 0 的整数。")
      return
    }
    if (
      draft.scheduledPowerOffEnabled &&
      draft.scheduledPowerOffStartMinute === draft.scheduledPowerOffEndMinute
    ) {
      setError("断电开始和结束时间不能相同。")
      return
    }
    if (
      !Number.isInteger(draft.powerRestoreJitterMinutes) ||
      draft.powerRestoreJitterMinutes < 0 ||
      draft.powerRestoreJitterMinutes > 60
    ) {
      setError("供电恢复错峰时间需要在 0–60 分钟之间。")
      return
    }
    setError("")
    await commit((d) => {
      d.settings = draft
      addAudit(d, "更新系统策略", "注册、提醒与数据保留")
    }, "系统策略已保存，原型内立即生效。")
  }
  const numeric = (
    key: keyof RegistrationSettings,
    label: string,
    hint?: string
  ) => (
    <Field label={label} hint={hint}>
      <PInput
        type="number"
        min={key === "powerRestoreJitterMinutes" ? 0 : 1}
        max={10000}
        value={String(draft[key])}
        onChange={(e) => number(key, e.target.value)}
      />
    </Field>
  )
  const timeInput = (
    key: "scheduledPowerOffStartMinute" | "scheduledPowerOffEndMinute",
    label: string
  ) => (
    <Field label={label}>
      <PInput
        type="time"
        required
        value={`${String(Math.floor(draft[key] / 60)).padStart(2, "0")}:${String(draft[key] % 60).padStart(2, "0")}`}
        onChange={(e) => {
          const [h, m] = e.target.value.split(":").map(Number)
          setDraft((d) => ({ ...d, [key]: h * 60 + m }))
        }}
      />
    </Field>
  )
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="系统策略"
        description="控制注册范围、远端访问频率与数据保留。"
      />
      <form className="cp-policies-form" onSubmit={save}>
        <SectionHeading title="注册与默认权限" />
        <SettingRow title="开放注册" description="关闭后只允许管理员创建用户。">
          <Toggle
            label="开放注册"
            checked={draft.openRegistration}
            onChange={(v) => toggle("openRegistration", v)}
          />
        </SettingRow>
        <SettingRow
          title="注册需要邀请码"
          description="使用有效邀请码的用户才可以创建账户。"
        >
          <Toggle
            label="注册需要邀请码"
            checked={draft.inviteRequired}
            disabled={!draft.openRegistration}
            onChange={(v) => toggle("inviteRequired", v)}
          />
        </SettingRow>
        <SettingRow
          title="新用户允许刷新"
          description="新建账户的默认远端刷新权限。"
        >
          <Toggle
            label="新用户允许刷新"
            checked={draft.defaultRefreshEnabled}
            onChange={(v) => toggle("defaultRefreshEnabled", v)}
          />
        </SettingRow>
        <div className="cp-form-grid">
          {numeric("defaultDeviceLimit", "新用户设备上限 / 台")}
        </div>
        <SectionHeading title="按需提醒" />
        <SettingRow
          title="允许后台空闲提醒"
          description="只有用户主动创建临时提醒后才会检查，不运行固定周期规则。"
        >
          <Toggle
            label="允许后台空闲提醒"
            checked={draft.backgroundRemindersEnabled}
            onChange={(v) => toggle("backgroundRemindersEnabled", v)}
          />
        </SettingRow>
        <div className="cp-form-grid">
          {numeric("watchRefreshIntervalMinutes", "检查间隔 / 分钟")}
          {numeric("watchPileLimitPerUser", "每人同时提醒的充电桩 / 台")}
          {numeric("watchDailyRefreshQuota", "每人每日检查额度 / 次")}
        </div>
        <SectionHeading title="计划断电" />
        <SettingRow
          title="在断电时段暂停请求"
          description="临时提醒的截止时间不超过当日断电开始时间。"
        >
          <Toggle
            label="在断电时段暂停请求"
            checked={draft.scheduledPowerOffEnabled}
            onChange={(v) => toggle("scheduledPowerOffEnabled", v)}
          />
        </SettingRow>
        {draft.scheduledPowerOffEnabled && (
          <div className="cp-form-grid">
            {timeInput("scheduledPowerOffStartMinute", "断电开始")}
            {timeInput("scheduledPowerOffEndMinute", "恢复供电")}
            {numeric(
              "powerRestoreJitterMinutes",
              "恢复供电后错峰 / 分钟",
              "避免所有账户在同一时间请求。"
            )}
            <Field label="时区">
              <select
                className="cp-input"
                value={draft.scheduledPowerOffTimezone}
                onChange={(e) =>
                  setDraft((d) => ({
                    ...d,
                    scheduledPowerOffTimezone: e.target.value,
                  }))
                }
              >
                <option value="Asia/Shanghai">亚洲 / 上海（UTC+8）</option>
              </select>
            </Field>
          </div>
        )}
        <SectionHeading
          title="数据保留"
          description="待处理问题持续保留，已解决通知按策略清理。"
        />
        <div className="cp-form-grid">
          {numeric("statsRetentionDays", "运行统计 / 天")}
          {numeric("portHistoryRetentionDays", "端口历史 / 天")}
          {numeric("notificationRetentionDays", "已解决通知 / 天")}
        </div>
        <FormError message={error} />
        <div className="cp-save-bar">
          <span>{changed ? "有尚未保存的更改" : "所有更改已保存"}</span>
          <div className="cp-row-actions">
            <PButton
              tone="quiet"
              disabled={!changed}
              onClick={() => {
                setDraft(data.settings)
                setError("")
              }}
            >
              撤销更改
            </PButton>
            <SaveButton />
          </div>
        </div>
      </form>
    </div>
  )
}

function InvitesPage() {
  const { data, openModal, commit, addAudit, notify, offline, scenario } =
    usePrototype()
  const [query, setQuery] = useState("")
  const items = (scenario === "empty" ? [] : data.invites).filter((i) =>
    i.code.toLowerCase().includes(query.toLowerCase())
  )
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="邀请注册"
        description="把有效邀请码分享给需要使用的人。"
        actions={
          <PButton
            tone="brand"
            disabled={offline}
            onClick={() => openModal({ kind: "invite" })}
          >
            <Plus size={16} />
            创建邀请码
          </PButton>
        }
      />
      <div className="cp-page-rule">
        <SearchField
          label="搜索邀请码"
          value={query}
          onChange={setQuery}
          placeholder="搜索邀请码"
        />
        <span className="cp-small cp-muted">
          {data.settings.inviteRequired
            ? "当前注册需要邀请码"
            : "当前开放注册，无需邀请码"}
        </span>
      </div>
      {!items.length ? (
        <EmptyState
          icon={Users}
          title="还没有可显示的邀请码"
          description="创建一个邀请码，邀请同伴一起使用。"
          action={
            <PButton onClick={() => openModal({ kind: "invite" })}>
              创建邀请码
            </PButton>
          }
        />
      ) : (
        <div className="cp-table-wrap">
          <table className="cp-data-table">
            <thead>
              <tr>
                <th>邀请码</th>
                <th>状态</th>
                <th>已使用</th>
                <th>有效期至</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((i) => (
                <tr key={i.id}>
                  <td className="cp-mono">
                    <strong>{i.code}</strong>
                  </td>
                  <td>
                    <Badge tone={i.enabled ? "idle" : "neutral"}>
                      {i.enabled ? "可使用" : "已停用"}
                    </Badge>
                  </td>
                  <td>{i.uses} 次</td>
                  <td>{i.expires || "长期有效"}</td>
                  <td>
                    <div className="cp-row-actions">
                      <PButton
                        tone="quiet"
                        aria-label={`复制 ${i.code}`}
                        onClick={async () => {
                          try {
                            await navigator.clipboard.writeText(i.code)
                            notify("邀请码已复制。")
                          } catch {
                            notify(`请手动复制：${i.code}`, "info")
                          }
                        }}
                      >
                        <Copy size={14} />
                        复制
                      </PButton>
                      <PButton
                        disabled={offline}
                        tone="quiet"
                        onClick={() =>
                          void commit(
                            (d) => {
                              const item = d.invites.find((x) => x.id === i.id)
                              if (item) item.enabled = !item.enabled
                              addAudit(
                                d,
                                i.enabled ? "停用邀请码" : "启用邀请码",
                                i.code
                              )
                            },
                            i.enabled ? "邀请码已停用。" : "邀请码已启用。"
                          )
                        }
                      >
                        {i.enabled ? "停用" : "启用"}
                      </PButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
function AuditPage() {
  const { data, scenario } = usePrototype()
  const [query, setQuery] = useState("")
  const [result, setResult] = useState("all")
  const [page, setPage] = useState(1)
  const items = (scenario === "empty" ? [] : data.audit)
    .filter((a) => `${a.action} ${a.target}`.includes(query.trim()))
    .filter((a) => result === "all" || a.result === result)
  const pages = Math.max(1, Math.ceil(items.length / 8))
  const current = Math.min(page, pages)
  return (
    <div className="cp-standard-page">
      <PageHeading
        title="操作记录"
        description="管理操作留下记录，重要变化有据可查。"
      />
      <div className="cp-page-rule">
        <SearchField
          label="搜索操作记录"
          placeholder="搜索操作或对象"
          value={query}
          onChange={(v) => {
            setQuery(v)
            setPage(1)
          }}
        />
        <select
          className="cp-input cp-select-compact"
          aria-label="操作结果"
          value={result}
          onChange={(e) => {
            setResult(e.target.value)
            setPage(1)
          }}
        >
          <option value="all">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
        </select>
      </div>
      {!items.length ? (
        <EmptyState
          icon={ClipboardList}
          title="没有匹配的记录"
          description="调整查询条件，或完成一次管理操作后再回来。"
        />
      ) : (
        <div className="cp-table-wrap">
          <table className="cp-data-table">
            <thead>
              <tr>
                <th>操作</th>
                <th>对象</th>
                <th>操作人</th>
                <th>结果</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {items.slice((current - 1) * 8, current * 8).map((a) => (
                <tr key={a.id}>
                  <td>
                    <strong>{a.action}</strong>
                  </td>
                  <td>{a.target}</td>
                  <td>admin</td>
                  <td>
                    <Badge
                      tone={a.result === "success" ? "idle" : "danger"}
                      dot={false}
                    >
                      {a.result === "success" ? "成功" : "失败"}
                    </Badge>
                  </td>
                  <td>{a.time}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="cp-pagination">
        <span>
          共 {items.length} 条 · 第 {current} / {pages} 页
        </span>
        <div>
          <PButton
            tone="quiet"
            disabled={current <= 1}
            onClick={() => setPage(current - 1)}
          >
            上一页
          </PButton>
          <PButton
            tone="quiet"
            disabled={current >= pages}
            onClick={() => setPage(current + 1)}
          >
            下一页
          </PButton>
        </div>
      </div>
    </div>
  )
}
