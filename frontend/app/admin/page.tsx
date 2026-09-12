"use client"

import { PlusIcon, RefreshCwIcon } from "lucide-react"
import dynamic from "next/dynamic"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useState } from "react"
import { AppShell, type WorkbenchSection } from "@/components/app-shell"
import { AdminHealthStatus } from "@/components/admin-health-status"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import { useOnline } from "@/lib/browser-state"
import { adminApi } from "@/lib/admin-api"
import { notify } from "@/lib/feedback"
import { updatePageQuery } from "@/lib/workbench"
import type { AdminHealth, AdminStats, UserRole } from "@/lib/types"
import type { AdminTrendRange, AdminTrendsResponse } from "@/lib/api/generated"
import {
  WorkbenchButton,
  WorkbenchError,
  WorkbenchLoading,
} from "@/components/workbench/surfaces"

const loading = () => <WorkbenchLoading />
const AdminOverview = dynamic(
  () =>
    import("@/components/admin-overview").then(
      (module) => module.AdminOverview
    ),
  { ssr: false, loading }
)
const AdminIncidents = dynamic(
  () =>
    import("@/components/admin-incident-panel").then(
      (module) => module.AdminIncidentPanel
    ),
  { ssr: false, loading }
)
const AdminUsers = dynamic(
  () =>
    import("@/components/workbench/admin-users-page").then(
      (module) => module.WorkbenchAdminUsers
    ),
  { ssr: false, loading }
)
const Operations = dynamic(
  () =>
    import("@/components/workbench/admin-system-pages").then(
      (module) => module.WorkbenchOperations
    ),
  { ssr: false, loading }
)
const Policies = dynamic(
  () =>
    import("@/components/workbench/admin-system-pages").then(
      (module) => module.WorkbenchPolicies
    ),
  { ssr: false, loading }
)
const Invites = dynamic(
  () =>
    import("@/components/workbench/admin-system-pages").then(
      (module) => module.WorkbenchInvites
    ),
  { ssr: false, loading }
)
const Audit = dynamic(
  () =>
    import("@/components/workbench/admin-system-pages").then(
      (module) => module.WorkbenchAudit
    ),
  { ssr: false, loading }
)
const pages = {
  overview: ["运行概览", "先处理需要关注的事，再了解系统运行情况。"],
  incidents: ["异常处理", "定位影响、记录处理过程，再关闭问题。"],
  users: ["用户", "账户访问、设备额度与连接问题，在同一处处理。"],
  operations: ["系统运行", "服务、提醒调度、消息投递与数据保存情况。"],
  settings: ["系统策略", "控制注册范围、远端访问频率与数据保留。"],
  invites: ["邀请注册", "创建并分享有效邀请码，管理可注册的范围。"],
  audit: ["操作记录", "管理操作留下记录，重要变化有据可查。"],
} as const
type Tab = keyof typeof pages
export default function AdminPage() {
  return (
    <Suspense fallback={<WorkbenchLoading />}>
      <AdminContent />
    </Suspense>
  )
}
function AdminContent() {
  const router = useRouter(),
    params = useSearchParams(),
    online = useOnline()
  const { currentUser, fetchMe } = useAuth()
  const rawTab = params.get("tab"),
    tab: Tab =
      rawTab && Object.hasOwn(pages, rawTab) ? (rawTab as Tab) : "overview"
  const range: AdminTrendRange =
    params.get("range") === "7d"
      ? "7d"
      : params.get("range") === "30d"
        ? "30d"
        : "24h"
  const [authError, setAuthError] = useState("")
  const [health, setHealth] = useState<AdminHealth | null>(null)
  const [healthError, setHealthError] = useState("")
  const [revision, setRevision] = useState(0)
  const [createOpen, setCreateOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [formError, setFormError] = useState("")
  const [form, setForm] = useState({
    username: "",
    password: "",
    role: "user" as UserRole,
  })
  const authorize = useCallback(async () => {
    try {
      const user = currentUser ?? (await fetchMe())
      if (!user) router.replace("/login")
      else if (user.role !== "admin") router.replace("/dashboard")
    } catch (reason) {
      setAuthError(
        reason instanceof Error ? reason.message : "暂时无法验证登录状态。"
      )
    }
  }, [currentUser, fetchMe, router])
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active) void authorize()
    })
    return () => {
      active = false
    }
  }, [authorize])
  useEffect(() => {
    if (currentUser?.role !== "admin") return
    let active = true
    adminApi
      .health()
      .then((value) => {
        if (active) {
          setHealth(value)
          setHealthError("")
        }
      })
      .catch((reason) => {
        if (active)
          setHealthError(
            reason instanceof Error ? reason.message : "健康检查暂时不可用。"
          )
      })
    return () => {
      active = false
    }
  }, [currentUser?.role, revision])
  async function create(event: React.FormEvent) {
    event.preventDefault()
    if (form.username.trim().length < 3 || form.password.length < 8) {
      setFormError("用户名至少 3 位，初始密码至少 8 位。")
      return
    }
    setPending(true)
    setFormError("")
    try {
      const user = await adminApi.createUser({
        ...form,
        username: form.username.trim(),
      })
      setCreateOpen(false)
      setForm({ username: "", password: "", role: "user" })
      notify.success("用户已创建")
      router.push(`/admin?tab=users&user=${encodeURIComponent(user.id)}`)
    } catch (reason) {
      setFormError(
        reason instanceof Error ? reason.message : "创建用户失败，请重试。"
      )
    } finally {
      setPending(false)
    }
  }
  if (currentUser?.role !== "admin")
    return (
      <AppShell title="管理工作区" description="正在确认访问权限。">
        {authError ? (
          <WorkbenchError message={authError} retry={() => void authorize()} />
        ) : (
          <WorkbenchLoading label="正在验证账户" />
        )}
      </AppShell>
    )
  const openUser = (id: string) =>
    router.push(
      `/admin?tab=users&user=${encodeURIComponent(id)}&section=diagnostics`
    )
  return (
    <AppShell
      title={pages[tab][0]}
      description={pages[tab][1]}
      activeSection={tab as WorkbenchSection}
      actions={
        <>
          {tab === "users" && (
            <WorkbenchButton
              disabled={!online}
              onClick={() => {
                setFormError("")
                setCreateOpen(true)
              }}
            >
              <PlusIcon size={15} />
              创建用户
            </WorkbenchButton>
          )}
          <WorkbenchButton
            variant="outline"
            disabled={!online}
            onClick={() => setRevision((value) => value + 1)}
          >
            <RefreshCwIcon size={15} />
            更新状态
          </WorkbenchButton>
        </>
      }
    >
      <div className="wb-standard-page">
        {(tab === "overview" || tab === "operations") && (
          <div className="wb-admin-healthline">
            <AdminHealthStatus
              health={health}
              onRecheck={async () => setHealth(await adminApi.health())}
            />
            <span>
              {healthError || "主服务、数据库与扫码服务的最近检查结果"}
            </span>
          </div>
        )}
        <div key={`${tab}-${revision}`}>
          {tab === "overview" ? (
            <OverviewData range={range} onUser={openUser} />
          ) : tab === "incidents" ? (
            <div className="wb-continuous">
              <AdminIncidents onUser={openUser} />
            </div>
          ) : tab === "users" ? (
            <AdminUsers currentUserId={currentUser.id} />
          ) : tab === "operations" ? (
            <Operations />
          ) : tab === "settings" ? (
            <Policies />
          ) : tab === "invites" ? (
            <Invites />
          ) : (
            <Audit />
          )}
        </div>
        <Dialog
          open={createOpen}
          onOpenChange={(open) => !pending && setCreateOpen(open)}
        >
          <DialogContent className="workbench-dialog">
            <DialogHeader>
              <DialogTitle>创建用户</DialogTitle>
              <DialogDescription>
                每个账户独立保存设备和凭据。管理员可以维护其他账户与系统策略。
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={create}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="new-username">用户名</FieldLabel>
                  <Input
                    id="new-username"
                    required
                    autoComplete="off"
                    minLength={3}
                    value={form.username}
                    onChange={(event) =>
                      setForm({ ...form, username: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="new-password">初始密码</FieldLabel>
                  <Input
                    id="new-password"
                    type="password"
                    autoComplete="new-password"
                    required
                    minLength={8}
                    value={form.password}
                    onChange={(event) =>
                      setForm({ ...form, password: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="new-role">账户角色</FieldLabel>
                  <select
                    id="new-role"
                    className="wb-input"
                    value={form.role}
                    onChange={(event) =>
                      setForm({ ...form, role: event.target.value as UserRole })
                    }
                  >
                    <option value="user">普通用户</option>
                    <option value="admin">管理员</option>
                  </select>
                </Field>
                {formError && (
                  <p className="wb-form-error" role="alert">
                    {formError}
                  </p>
                )}
                <DialogFooter>
                  <WorkbenchButton
                    type="button"
                    variant="outline"
                    disabled={pending}
                    onClick={() => setCreateOpen(false)}
                  >
                    取消
                  </WorkbenchButton>
                  <WorkbenchButton
                    type="submit"
                    busy={pending}
                    disabled={!online}
                  >
                    确认创建
                  </WorkbenchButton>
                </DialogFooter>
              </FieldGroup>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </AppShell>
  )
}

function OverviewData({
  range,
  onUser,
}: {
  range: AdminTrendRange
  onUser: (id: string) => void
}) {
  const [stats, setStats] = useState<AdminStats | null>(null),
    [trends, setTrends] = useState<AdminTrendsResponse | null>(null)
  const [error, setError] = useState<string | null>(null),
    [pending, setPending] = useState(true),
    [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    Promise.allSettled([adminApi.stats(), adminApi.trends(range)]).then(
      ([s, t]) => {
        if (!active) return
        if (s.status === "fulfilled") setStats(s.value)
        if (t.status === "fulfilled") {
          setTrends(t.value)
          setError(null)
        } else
          setError(
            t.reason instanceof Error ? t.reason.message : "趋势暂时无法读取。"
          )
        if (s.status === "rejected")
          notify.error(s.reason, { title: "运行概览暂时无法读取" })
        setPending(false)
      }
    )
    return () => {
      active = false
    }
  }, [range, revision])
  return (
    <AdminOverview
      stats={stats}
      trends={trends}
      trendRange={range}
      trendLoading={pending}
      trendError={error}
      onTrendRangeChange={(value) => {
        setPending(true)
        updatePageQuery({ range: value === "24h" ? null : value })
      }}
      onTrendReload={() => {
        setPending(true)
        setRevision((value) => value + 1)
      }}
      onUser={onUser}
    />
  )
}
