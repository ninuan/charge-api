"use client"

import { ActivityIcon, CircleAlertIcon, PlusIcon, RefreshCwIcon, Settings2Icon, ShieldCheckIcon, Trash2Icon, UsersIcon } from "lucide-react"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useEffect, useState } from "react"
import { toast } from "sonner"

import { AdminUserFilters } from "@/components/admin-user-filters"
import { AdminUserDiagnostics } from "@/components/admin-user-diagnostics"
import { AdminUserRefreshTiming } from "@/components/admin-user-refresh-timing"
import { AppShell } from "@/components/app-shell"
import { MetricCard } from "@/components/metric-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useAuth } from "@/lib/auth-context"
import { adminApi } from "@/lib/admin-api"
import type { AdminHealth, AdminStats, AdminUserListQuery, AdminUserPage, CredentialState, InviteCode, RegistrationSettings, UserRole } from "@/lib/types"

type Tab = "overview" | "users" | "settings"

const emptyQuery: AdminUserListQuery = { page: 1, pageSize: 15, search: "", account: "all", credential: "all", health: "all" }
const titles = {
  overview: ["运营总览", "将待处理问题、服务健康和最近异常集中在一个紧凑看板中。"],
  users: ["用户管理", "按账户状态、扫码凭据和设备健康度定位需要处理的账户。"],
  settings: ["系统设置", "管理注册策略、新账户默认权限和长期邀请码。"],
} as const

const credentialText: Record<CredentialState, string> = {
  unbound: "未绑定扫码",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "凭据同步失败",
  expired: "凭据已失效",
}

function healthLabel(state: "healthy" | "degraded" | "unavailable") {
  return state === "healthy" ? "正常" : state === "degraded" ? "异常" : "不可用"
}

function credentialVariant(state: CredentialState) {
  return state === "healthy" || state === "waiting_device" ? "secondary" : "destructive"
}

function issueTypeLabel(type: string) {
  return ({ credential: "凭据", cookie_expired: "凭据失效", refresh: "刷新", stale: "数据滞后", offline: "离线端口", operation: "用户操作" } as Record<string, string>)[type] ?? "系统"
}

export default function AdminPage() {
  return <Suspense fallback={<div className="min-h-dvh bg-muted/35" />}><AdminPageContent /></Suspense>
}

function AdminPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { currentUser, fetchMe } = useAuth()
  const currentTab = searchParams.get("tab")
  const tab = (["overview", "users", "settings"].includes(currentTab ?? "") ? currentTab : "overview") as Tab
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [health, setHealth] = useState<AdminHealth | null>(null)
  const [userPage, setUserPage] = useState<AdminUserPage | null>(null)
  const [query, setQuery] = useState(emptyQuery)
  const [settings, setSettings] = useState<RegistrationSettings | null>(null)
  const [invites, setInvites] = useState<InviteCode[]>([])
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ username: "", password: "", role: "user" as UserRole })

  const setTab = (next: Tab) => router.replace(next === "overview" ? "/admin" : `/admin?tab=${next}`)
  async function loadOverview() { const [nextStats, nextHealth] = await Promise.all([adminApi.stats(), adminApi.health()]); setStats(nextStats); setHealth(nextHealth) }
  async function loadUsers(nextQuery = query) { const page = await adminApi.users(nextQuery); setUserPage(page); setQuery({ ...nextQuery, page: page.page, pageSize: page.pageSize }) }
  async function loadSettings() { const [nextSettings, nextInvites] = await Promise.all([adminApi.settings(), adminApi.invites()]); setSettings(nextSettings); setInvites(Array.isArray(nextInvites) ? nextInvites : []) }
  async function load() { setLoading(true); try { if (tab === "overview") await loadOverview(); if (tab === "users") await loadUsers(); if (tab === "settings") await loadSettings() } catch (reason) { toast.error((reason as Error).message) } finally { setLoading(false) } }

  useEffect(() => {
    void (async () => {
      const user = currentUser ?? await fetchMe()
      if (!user) return router.replace("/login")
      if (user.role !== "admin") return router.replace("/dashboard")
      await load()
    })()
  // Initial access check intentionally follows the selected tab only.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  async function createUser(event: React.FormEvent) {
    event.preventDefault()
    if (form.username.trim().length < 3 || form.password.length < 8) return toast.error("用户名至少 3 位，密码至少 8 位")
    try {
      await adminApi.createUser({ ...form, username: form.username.trim() })
      setCreateOpen(false)
      setForm({ username: "", password: "", role: "user" })
      toast.success("用户已创建")
      await loadUsers({ ...query, page: 1 })
    } catch (reason) { toast.error((reason as Error).message) }
  }

  return <AppShell compact title={titles[tab][0]} description={titles[tab][1]} actions={<div className="grid grid-cols-2 gap-2 [&_button]:w-full md:flex md:[&_button]:w-auto"><Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogTrigger render={<Button><PlusIcon />创建用户</Button>} /><DialogContent><DialogHeader><DialogTitle>创建用户</DialogTitle><DialogDescription>普通用户管理扫码登录和充电桩；管理员可维护账户与系统策略。</DialogDescription></DialogHeader><form onSubmit={createUser}><FieldGroup><Field><FieldLabel htmlFor="new-username">用户名</FieldLabel><Input id="new-username" value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} /></Field><Field><FieldLabel htmlFor="new-password">初始密码</FieldLabel><Input id="new-password" type="password" value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></Field><Field><FieldLabel htmlFor="new-role">角色</FieldLabel><select id="new-role" className="h-8 rounded-lg border bg-background px-2.5 text-sm" value={form.role} onChange={(event) => setForm({ ...form, role: event.target.value as UserRole })}><option value="user">普通用户</option><option value="admin">管理员</option></select></Field><DialogFooter><Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>取消</Button><Button type="submit">确认创建</Button></DialogFooter></FieldGroup></form></DialogContent></Dialog><Button variant="outline" disabled={loading} onClick={() => void load()}><RefreshCwIcon className={loading ? "animate-spin" : ""} />刷新数据</Button></div>}>
    <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}><TabsList className="mb-4"><TabsTrigger value="overview"><ActivityIcon />运营总览</TabsTrigger><TabsTrigger value="users"><UsersIcon />用户管理</TabsTrigger><TabsTrigger value="settings"><Settings2Icon />系统设置</TabsTrigger></TabsList></Tabs>
    {tab === "overview" && <Overview stats={stats} health={health} onUsers={() => setTab("users")} />}
    {tab === "users" && <Users page={userPage} query={query} load={loadUsers} />}
    {tab === "settings" && settings && <Settings settings={settings} setSettings={setSettings} invites={invites} reload={loadSettings} />}
  </AppShell>
}

function Overview({ stats, health, onUsers }: { stats: AdminStats | null; health: AdminHealth | null; onUsers: () => void }) {
  const overview = stats?.overview
  return <div className="space-y-4"><section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5"><MetricCard compact label="待处理问题" value={overview?.openIssues ?? 0} detail="需要关注的异常" icon={CircleAlertIcon} /><MetricCard compact label="远端成功率" value={Math.round(overview?.remoteSuccessRate ?? 0)} detail="近期远端请求成功率（%）" icon={ActivityIcon} /><MetricCard compact label="活跃用户" value={overview?.activeUsers ?? 0} detail="近期访问过的账户" icon={UsersIcon} /><MetricCard compact label="管理设备" value={overview?.managedDevices ?? 0} detail="系统中的充电桩" icon={ShieldCheckIcon} /><MetricCard compact label="离线端口" value={overview?.offlinePorts ?? 0} detail="需要复查设备状态" icon={CircleAlertIcon} /></section><Card className="shadow-none"><CardHeader className="pb-3"><CardTitle className="text-base">服务健康</CardTitle><p className="text-xs text-muted-foreground">服务状态仅描述可用性，不会暴露上游连接细节。</p></CardHeader><CardContent className="grid gap-2 sm:grid-cols-3">{(["charge", "database", "yyb"] as const).map((name) => <div key={name} className="flex items-center justify-between gap-3 rounded-lg border p-3"><div><p className="text-sm font-medium">{{ charge: "充电服务", database: "数据库", yyb: "扫码服务" }[name]}</p><p className="mt-1 text-xs text-muted-foreground">{health?.[name].message ?? "正在检查服务状态"}</p></div><Badge variant={health?.[name].state === "healthy" ? "secondary" : "destructive"}>{health ? healthLabel(health[name].state) : "检查中"}</Badge></div>)}</CardContent></Card><Card className="shadow-none"><CardHeader className="flex-row items-center justify-between pb-3"><div><CardTitle className="text-base">最近异常</CardTitle><p className="mt-1 text-xs text-muted-foreground">管理员诊断已脱敏，不含登录凭据、会话或上游地址。</p></div><Button variant="outline" size="sm" onClick={onUsers}>筛选用户</Button></CardHeader><CardContent className="space-y-2">{stats?.exceptions?.length ? stats.exceptions.slice(0, 6).map((issue) => <div key={issue.id} className="flex flex-col gap-1 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"><div><p className="text-sm font-medium">{issue.username} · {issueTypeLabel(issue.type)}</p><p className="mt-1 text-xs text-muted-foreground">{issue.message}{issue.deviceId ? ` · 设备尾号 ${issue.deviceId}` : ""}</p></div><p className="text-xs text-muted-foreground">{new Date(issue.time).toLocaleString("zh-CN")}</p></div>) : <p className="py-4 text-sm text-muted-foreground">暂无近期异常。</p>}</CardContent></Card></div>
}

function Users({ page, query, load }: { page: AdminUserPage | null; query: AdminUserListQuery; load: (query?: AdminUserListQuery) => Promise<void> }) {
  async function update(id: string, payload: Parameters<typeof adminApi.updateUser>[1]) { try { await adminApi.updateUser(id, payload); toast.success("用户已更新"); await load() } catch (reason) { toast.error((reason as Error).message) } }
  return <div className="space-y-4"><AdminUserFilters query={query} onApply={load} /><div className="flex items-center justify-between px-1"><p className="text-xs text-muted-foreground">{page ? `共找到 ${page.total} 位用户` : "正在加载用户目录"}</p><p className="text-xs text-muted-foreground">风险账户包含凭据异常、刷新失败、离线端口或已停用账户。</p></div><div className="grid gap-3">{page?.items.map((summary) => { const risk = !summary.user.enabled || summary.credential.state === "unbound" || summary.credential.state === "sync_failed" || summary.credential.state === "expired" || summary.lastRefresh.failedDevices > 0 || summary.dashboard.offlinePorts > 0; return <Card key={summary.user.id} className="shadow-none"><CardContent className="space-y-4 p-4"><div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-medium">{summary.user.username}</p><Badge variant={summary.user.enabled ? "secondary" : "destructive"}>{summary.user.enabled ? "已启用" : "已停用"}</Badge><Badge variant={credentialVariant(summary.credential.state)}>{credentialText[summary.credential.state]}</Badge>{risk && <Badge variant="destructive">需要关注</Badge>}</div><p className="mt-2 text-xs leading-5 text-muted-foreground">{summary.user.role === "admin" ? "管理员账户" : "普通用户"} · 设备额度 {summary.user.deviceLimit} · 已添加 {summary.dashboard.pileCount} 台设备 · 离线端口 {summary.dashboard.offlinePorts} 个</p><AdminUserRefreshTiming bound={summary.credential.bound} lastCheckedAt={summary.credential.lastCheckedAt} lastRemoteAt={summary.lastRefresh.lastRemoteAt} /></div><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => void update(summary.user.id, { enabled: !summary.user.enabled })}>{summary.user.enabled ? "停用" : "启用"}</Button><Button size="sm" variant="outline" onClick={() => void update(summary.user.id, { refreshEnabled: !summary.user.refreshEnabled })}>{summary.user.refreshEnabled ? "关闭刷新" : "开启刷新"}</Button><Button size="sm" variant="destructive" onClick={() => { if (window.confirm(`删除 ${summary.user.username}？`)) void adminApi.removeUser(summary.user.id).then(() => load()).catch((reason) => toast.error(reason.message)) }}><Trash2Icon />删除</Button></div></div><AdminUserDiagnostics diagnostics={summary.recoveryDiagnostics ?? []} /></CardContent></Card> })}{page && !page.items.length && <Card className="border-dashed shadow-none"><CardContent className="p-10 text-center text-sm text-muted-foreground">没有匹配的用户。可以重置筛选后重新查看。</CardContent></Card>}</div>{page && <div className="flex items-center justify-between"><p className="text-xs text-muted-foreground">第 {page.page}/{page.totalPages} 页</p><div className="flex gap-2"><Button size="sm" variant="outline" disabled={page.page <= 1} onClick={() => void load({ ...query, page: page.page - 1 })}>上一页</Button><Button size="sm" variant="outline" disabled={page.page >= page.totalPages} onClick={() => void load({ ...query, page: page.page + 1 })}>下一页</Button></div></div>}</div>
}

function Settings({ settings, setSettings, invites, reload }: { settings: RegistrationSettings; setSettings: (settings: RegistrationSettings) => void; invites: InviteCode[]; reload: () => Promise<void> }) {
  async function save(event: React.FormEvent) { event.preventDefault(); try { setSettings(await adminApi.saveSettings(settings)); toast.success("系统设置已保存") } catch (reason) { toast.error((reason as Error).message) } }
  const toggles = [["openRegistration", "开放自助注册", "关闭后仅能通过管理员创建账户或邀请码注册。"], ["inviteRequired", "注册需要邀请码", "仅在关闭公共注册时要求邀请码。"], ["defaultRefreshEnabled", "新账户默认允许刷新", "允许新账户主动向远端设备请求最新状态。"]] as const
  return <div className="grid gap-4 lg:grid-cols-2"><Card className="shadow-none"><CardHeader><CardTitle className="text-base">注册策略</CardTitle><p className="text-xs text-muted-foreground">这些规则只影响之后创建或注册的账户。</p></CardHeader><CardContent><form onSubmit={save}><FieldGroup>{toggles.map(([key, label, description]) => <label key={key} className="flex items-start justify-between gap-3 rounded-lg border p-3 text-sm"><span><span className="font-medium">{label}</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">{description}</span></span><input type="checkbox" checked={settings[key]} onChange={(event) => setSettings({ ...settings, [key]: event.target.checked })} /></label>)}<Field><FieldLabel htmlFor="device-limit">默认设备额度</FieldLabel><Input id="device-limit" type="number" value={settings.defaultDeviceLimit} onChange={(event) => setSettings({ ...settings, defaultDeviceLimit: Number(event.target.value) })} /><p className="text-xs text-muted-foreground">每个新普通用户最多可添加的充电桩数量。</p></Field><Field><FieldLabel htmlFor="retention">统计保留天数</FieldLabel><Input id="retention" type="number" value={settings.statsRetentionDays} onChange={(event) => setSettings({ ...settings, statsRetentionDays: Number(event.target.value) })} /><p className="text-xs text-muted-foreground">用于运营总览和异常分析的历史数据保留时间。</p></Field><Button type="submit">保存设置</Button></FieldGroup></form></CardContent></Card><Card className="shadow-none"><CardHeader className="flex-row items-start justify-between"><div><CardTitle className="text-base">邀请码</CardTitle><p className="mt-1 text-xs text-muted-foreground">邀请码仅在关闭公共注册并启用邀请码要求时生效。</p></div><Button size="sm" onClick={() => void adminApi.createInvite().then(reload).catch((reason) => toast.error(reason.message))}><PlusIcon />生成邀请码</Button></CardHeader><CardContent className="space-y-2">{invites.map((invite) => <div key={invite.id} className="flex items-center justify-between gap-3 rounded-lg border p-3"><div><p className="font-mono text-sm">{invite.code}</p><p className="mt-1 text-xs text-muted-foreground">已使用 {invite.usedCount} 次{invite.expiresAt ? ` · 到期 ${new Date(invite.expiresAt).toLocaleDateString("zh-CN")}` : " · 永不过期"}</p></div><Button size="sm" variant="ghost" aria-label={`删除邀请码 ${invite.code}`} onClick={() => void adminApi.removeInvite(invite.id).then(reload).catch((reason) => toast.error(reason.message))}><Trash2Icon /></Button></div>)}{!invites.length && <p className="py-4 text-sm text-muted-foreground">暂无邀请码。生成后可用于受限注册。</p>}</CardContent></Card></div>
}
