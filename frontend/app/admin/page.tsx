"use client"

import {
  ActivityIcon,
  PlusIcon,
  RefreshCwIcon,
  Settings2Icon,
  UsersIcon,
} from "lucide-react"
import dynamic from "next/dynamic"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useEffect, useState } from "react"
import { toast } from "sonner"

import { AdminHealthStatus } from "@/components/admin-health-status"
import { AppShell } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { adminApi } from "@/lib/admin-api"
import { useAuth } from "@/lib/auth-context"
import type {
  AdminHealth,
  AdminStats,
  AdminUserListQuery,
  AdminUserPage,
  InviteCodePage,
  RegistrationSettings,
  UserRole,
} from "@/lib/types"

// 三个标签页组件按需加载：停留在总览时无需下载用户/设置页的代码。
const overviewFallback = () => (
  <div className="grid gap-4">
    <Skeleton className="h-28 w-full" />
    <Skeleton className="h-44 w-full" />
    <Skeleton className="h-44 w-full" />
  </div>
)
const usersFallback = () => (
  <div className="grid gap-4">
    <Skeleton className="h-32 w-full md:h-28" />
    <Skeleton className="h-64 w-full md:h-80" />
  </div>
)
const settingsFallback = () => (
  <div className="grid gap-4 lg:grid-cols-2">
    <Skeleton className="h-[32rem] w-full" />
    <div className="grid content-start gap-4">
      <Skeleton className="h-40 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  </div>
)
const AdminOverview = dynamic(
  () => import("@/components/admin-overview").then((m) => m.AdminOverview),
  { ssr: false, loading: overviewFallback }
)
const AdminUsers = dynamic(
  () => import("@/components/admin-users").then((m) => m.AdminUsers),
  { ssr: false, loading: usersFallback }
)
const AdminSettings = dynamic(
  () => import("@/components/admin-settings").then((m) => m.AdminSettings),
  { ssr: false, loading: settingsFallback }
)

type Tab = "overview" | "users" | "settings"

const emptyQuery: AdminUserListQuery = {
  page: 1,
  pageSize: 15,
  search: "",
  account: "all",
  credential: "all",
  health: "all",
}

const titles = {
  overview: [
    "运营总览",
    "将待处理问题、运行趋势和最近异常集中在一个紧凑看板中。",
  ],
  users: ["用户管理", "按账户状态、扫码凭据和设备健康度定位需要处理的账户。"],
  settings: ["系统设置", "管理注册策略、新账户默认权限和长期邀请码。"],
} as const

const roleOptions = [
  { value: "user", label: "普通用户" },
  { value: "admin", label: "管理员" },
]

export default function AdminPage() {
  return (
    <Suspense fallback={<div className="min-h-dvh bg-muted/35" />}>
      <AdminPageContent />
    </Suspense>
  )
}

function AdminPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { currentUser, fetchMe } = useAuth()
  const currentTab = searchParams.get("tab")
  const tab = (
    ["overview", "users", "settings"].includes(currentTab ?? "")
      ? currentTab
      : "overview"
  ) as Tab
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [health, setHealth] = useState<AdminHealth | null>(null)
  const [userPage, setUserPage] = useState<AdminUserPage | null>(null)
  const [query, setQuery] = useState(emptyQuery)
  const [settings, setSettings] = useState<RegistrationSettings | null>(null)
  const [invitePage, setInvitePage] = useState<InviteCodePage | null>(null)
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({
    username: "",
    password: "",
    role: "user" as UserRole,
  })

  const setTab = (next: Tab) =>
    router.replace(next === "overview" ? "/admin" : `/admin?tab=${next}`)

  async function loadOverview() {
    setStats(await adminApi.stats())
  }

  async function loadUsers(nextQuery = query) {
    const page = await adminApi.users(nextQuery)
    setUserPage(page)
    setQuery({ ...nextQuery, page: page.page, pageSize: page.pageSize })
  }

  async function loadSettings(page = 1) {
    const [nextSettings, nextInvites] = await Promise.all([
      adminApi.settings(),
      adminApi.invites({ page, pageSize: 20 }),
    ])
    setSettings(nextSettings)
    setInvitePage(nextInvites)
  }

  async function load() {
    setLoading(true)
    try {
      const tabLoad =
        tab === "overview"
          ? loadOverview()
          : tab === "users"
            ? loadUsers()
            : loadSettings()
      await Promise.all([
        adminApi.health().then((nextHealth) => setHealth(nextHealth)),
        tabLoad,
      ])
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void (async () => {
      const user = currentUser ?? (await fetchMe())
      if (!user) return router.replace("/login")
      if (user.role !== "admin") return router.replace("/dashboard")
      await load()
    })()
    // Initial access check intentionally follows the selected tab only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  async function createUser(event: React.FormEvent) {
    event.preventDefault()
    if (form.username.trim().length < 3 || form.password.length < 8)
      return toast.error("用户名至少 3 位，密码至少 8 位")

    try {
      await adminApi.createUser({ ...form, username: form.username.trim() })
      setCreateOpen(false)
      setForm({ username: "", password: "", role: "user" })
      toast.success("用户已创建")
      await loadUsers({ ...query, page: 1 })
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  return (
    <AppShell
      compact
      title={titles[tab][0]}
      description={titles[tab][1]}
      actions={
        <div className="grid grid-cols-2 gap-2 md:flex [&_button]:w-full md:[&_button]:w-auto">
          <AdminHealthStatus health={health} />
          <Dialog open={createOpen} onOpenChange={setCreateOpen}>
            <DialogTrigger
              render={
                <Button>
                  <PlusIcon />
                  创建用户
                </Button>
              }
            />
            <DialogContent>
              <DialogHeader>
                <DialogTitle>创建用户</DialogTitle>
                <DialogDescription>
                  普通用户管理扫码登录和充电桩；管理员可维护账户与系统策略。
                </DialogDescription>
              </DialogHeader>
              <form onSubmit={createUser}>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="new-username">用户名</FieldLabel>
                    <Input
                      id="new-username"
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
                      value={form.password}
                      onChange={(event) =>
                        setForm({ ...form, password: event.target.value })
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="new-role">角色</FieldLabel>
                    <Select
                      items={roleOptions}
                      value={form.role}
                      onValueChange={(value) =>
                        setForm({
                          ...form,
                          role: value as UserRole,
                        })
                      }
                    >
                      <SelectTrigger id="new-role" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {roleOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <DialogFooter>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setCreateOpen(false)}
                    >
                      取消
                    </Button>
                    <Button type="submit">确认创建</Button>
                  </DialogFooter>
                </FieldGroup>
              </form>
            </DialogContent>
          </Dialog>
          <Button
            className="col-span-2 md:col-span-1"
            variant="outline"
            disabled={loading}
            onClick={() => void load()}
          >
            <RefreshCwIcon className={loading ? "animate-spin" : ""} />
            刷新数据
          </Button>
        </div>
      }
    >
      <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)}>
        <TabsList className="mb-4">
          <TabsTrigger value="overview">
            <ActivityIcon />
            运营总览
          </TabsTrigger>
          <TabsTrigger value="users">
            <UsersIcon />
            用户管理
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings2Icon />
            系统设置
          </TabsTrigger>
        </TabsList>
      </Tabs>
      {tab === "overview" && (
        <AdminOverview stats={stats} onUsers={() => setTab("users")} />
      )}
      {tab === "users" && (
        <AdminUsers
          page={userPage}
          query={query}
          load={loadUsers}
          currentUserId={currentUser?.id}
        />
      )}
      {tab === "settings" &&
        (settings ? (
          <AdminSettings
            settings={settings}
            setSettings={setSettings}
            invitePage={invitePage}
            reload={loadSettings}
          />
        ) : (
          settingsFallback()
        ))}
    </AppShell>
  )
}
