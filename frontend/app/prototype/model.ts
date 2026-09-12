import type {
  CredentialState,
  Pile,
  Port,
  RegistrationSettings,
} from "@/lib/types"

export const STORAGE_KEY = "charge.prototype.v2"
export const SNAPSHOT_AT = "2026-09-10T18:42:00+08:00"
export type Scenario =
  | "ready"
  | "loading"
  | "empty"
  | "error"
  | "offline"
  | "expired"
  | "power-off"
  | "quota"
export type Role = "user" | "admin" | "guest"
export type Duration = "1h" | "2h" | "4h" | "until_power_off"
export type View =
  | "piles"
  | "reminders"
  | "notifications"
  | "account"
  | "login"
  | "register"
  | "admin"
  | "users"
  | "incidents"
  | "operations"
  | "policies"
  | "invites"
  | "audit"
export type Route = { view: View; id?: string; tab?: string; port?: number }
export type NoticeType =
  "pile_available" | "pile_offline" | "pile_recovered" | "credential_expired"

export interface DemoWatch {
  id: string
  deviceId: string
  duration: Duration
  status: "active" | "notified" | "expired" | "cancelled" | "legacy"
  createdAt: string
  expiresAt: string
  nextCheckAt: string
}
export interface DemoNotice {
  id: string
  deviceId?: string
  portId?: number
  type: NoticeType
  title: string
  message: string
  createdAt: string
  read: boolean
  resolved: boolean
  delivery: "submitted" | "queued" | "failed" | "suppressed" | "none"
}
export interface DemoUser {
  id: string
  username: string
  role: "admin" | "user"
  enabled: boolean
  deviceCount: number
  deviceLimit: number
  refreshEnabled: boolean
  credential: CredentialState
  lastActive: string
  requests: number
  sessions: number
}
export interface DemoIncident {
  id: string
  userId: string
  title: string
  detail: string
  level: "warning" | "critical"
  status: "open" | "acknowledged" | "resolved"
  occurrences: number
  time: string
  note: string
}
export interface DemoAudit {
  id: string
  action: string
  target: string
  time: string
  result: "success" | "failure"
}
export interface PrototypeData {
  version: 2
  demoRole: Role
  theme: "light" | "dark" | "system"
  reduceMotion: boolean
  username: string
  piles: Pile[]
  watches: DemoWatch[]
  notices: DemoNotice[]
  credential: CredentialState
  wxBound: boolean
  wxEnabled: boolean
  wxEvents: NoticeType[]
  wxTest: "none" | "queued" | "submitted" | "failed"
  browserEnabled: boolean
  quietEnabled: boolean
  quietStart: string
  quietEnd: string
  otherSessions: boolean
  users: DemoUser[]
  incidents: DemoIncident[]
  audit: DemoAudit[]
  invites: {
    id: string
    code: string
    enabled: boolean
    uses: number
    expires: string
  }[]
  settings: RegistrationSettings
  snapshotAt: string
}

function ports(statuses: Port["status"][]): Port[] {
  return statuses.map((status, i) => ({
    id: i + 1,
    status,
    powerKw: status === "in_use" ? [0.12, 0.18, 0.26, 0.31, 0.22][i % 5] : 0,
    energyKwh: status === "in_use" ? Number(((i + 2) * 0.083).toFixed(2)) : 0,
    updatedAt: SNAPSHOT_AT,
    startedAt: status === "in_use" ? "2026-09-10T16:10:00+08:00" : undefined,
    sessionMin: status === "in_use" ? 240 : 0,
    usedSeconds: status === "in_use" ? (52 + i * 17) * 60 : 0,
    usedText:
      status === "in_use"
        ? `${Math.floor((52 + i * 17) / 60)} 小时 ${(52 + i * 17) % 60} 分钟`
        : undefined,
    remainingText:
      status === "in_use"
        ? [
            "2 小时 08 分钟",
            "1 小时 46 分钟",
            "约 38 分钟",
            "2 小时 12 分钟",
            "约 56 分钟",
          ][i % 5]
        : undefined,
  }))
}

function pile(
  id: string,
  number: string,
  name: string,
  address: string,
  statuses: Port["status"][],
  sortOrder: number
): Pile {
  return {
    id,
    number,
    name,
    address,
    sortOrder,
    status: statuses.every((s) => s === "offline") ? "offline" : "online",
    online: statuses.some((s) => s !== "offline"),
    openNum: statuses.length,
    createdAt: "2026-09-01T12:00:00+08:00",
    updatedAt: SNAPSHOT_AT,
    source: "prototype-fixture",
    ports: ports(statuses),
    usedPortIds: statuses.flatMap((s, i) => (s === "in_use" ? [i + 1] : [])),
  }
}

export function initialData(): PrototypeData {
  return {
    version: 2,
    demoRole: "user",
    theme: "light",
    reduceMotion: false,
    username: "林一",
    snapshotAt: SNAPSHOT_AT,
    piles: [
      pile(
        "202609100001",
        "608201",
        "桂园 · 北门车棚",
        "北门入口右侧，第一排车棚",
        [
          "idle",
          "in_use",
          "idle",
          "in_use",
          "in_use",
          "idle",
          "in_use",
          "in_use",
          "idle",
          "offline",
        ],
        0
      ),
      pile(
        "202609100002",
        "608202",
        "桂园 · 3 号楼",
        "3 号楼东侧，非机动车停放区",
        [
          "in_use",
          "in_use",
          "in_use",
          "in_use",
          "in_use",
          "in_use",
          "in_use",
          "in_use",
        ],
        1
      ),
      pile(
        "202609100003",
        "608203",
        "桂园 · 南门车棚",
        "南门内侧，靠近快递柜",
        [
          "in_use",
          "idle",
          "in_use",
          "in_use",
          "idle",
          "in_use",
          "in_use",
          "idle",
        ],
        2
      ),
      pile(
        "202609100004",
        "608204",
        "科创园 · B 座",
        "B 座负一层，自行车入口旁",
        ["offline", "offline", "offline", "offline", "offline", "offline"],
        3
      ),
    ],
    watches: [
      {
        id: "watch-1",
        deviceId: "202609100002",
        duration: "2h",
        status: "active",
        createdAt: "2026-09-10T18:20:00+08:00",
        expiresAt: "2026-09-10T20:20:00+08:00",
        nextCheckAt: "2026-09-10T18:45:00+08:00",
      },
      {
        id: "watch-2",
        deviceId: "202609100001",
        duration: "1h",
        status: "notified",
        createdAt: "2026-09-10T17:40:00+08:00",
        expiresAt: "2026-09-10T18:40:00+08:00",
        nextCheckAt: "",
      },
      {
        id: "watch-3",
        deviceId: "202609100003",
        duration: "2h",
        status: "expired",
        createdAt: "2026-09-09T16:00:00+08:00",
        expiresAt: "2026-09-09T18:00:00+08:00",
        nextCheckAt: "",
      },
    ],
    notices: [
      {
        id: "notice-1",
        deviceId: "202609100001",
        portId: 3,
        type: "pile_available",
        title: "北门车棚有空闲口了",
        message: "3 号口已空闲，这次等待已结束。出发前可以再确认一下状态。",
        createdAt: "2026-09-10T18:35:00+08:00",
        read: false,
        resolved: false,
        delivery: "submitted",
      },
      {
        id: "notice-2",
        deviceId: "202609100004",
        type: "pile_offline",
        title: "科创园 B 座暂时无法连接",
        message:
          "连续 3 次未能读取状态，已保留最近快照。可以稍后刷新或检查现场供电。",
        createdAt: "2026-09-10T18:10:00+08:00",
        read: false,
        resolved: false,
        delivery: "submitted",
      },
      {
        id: "notice-3",
        deviceId: "202609100003",
        type: "pile_recovered",
        title: "南门车棚恢复连接",
        message: "已重新读取到充电口状态。",
        createdAt: "2026-09-10T16:24:00+08:00",
        read: true,
        resolved: false,
        delivery: "none",
      },
      {
        id: "notice-4",
        type: "credential_expired",
        title: "充电平台登录已恢复",
        message: "9 月 9 日的登录失效问题已解决，现在可以正常刷新。",
        createdAt: "2026-09-09T12:30:00+08:00",
        read: true,
        resolved: true,
        delivery: "none",
      },
    ],
    credential: "healthy",
    wxBound: true,
    wxEnabled: true,
    wxEvents: [
      "pile_available",
      "credential_expired",
      "pile_offline",
      "pile_recovered",
    ],
    wxTest: "none",
    browserEnabled: false,
    quietEnabled: true,
    quietStart: "23:00",
    quietEnd: "07:00",
    otherSessions: true,
    users: [
      {
        id: "user-1",
        username: "linyi",
        role: "user",
        enabled: true,
        deviceCount: 4,
        deviceLimit: 10,
        refreshEnabled: true,
        credential: "healthy",
        lastActive: "18:42",
        requests: 36,
        sessions: 2,
      },
      {
        id: "user-2",
        username: "chenmo",
        role: "user",
        enabled: true,
        deviceCount: 3,
        deviceLimit: 10,
        refreshEnabled: true,
        credential: "expired",
        lastActive: "18:38",
        requests: 18,
        sessions: 1,
      },
      {
        id: "user-3",
        username: "zhouzhou",
        role: "user",
        enabled: true,
        deviceCount: 2,
        deviceLimit: 10,
        refreshEnabled: true,
        credential: "healthy",
        lastActive: "17:20",
        requests: 24,
        sessions: 1,
      },
      {
        id: "user-4",
        username: "xiaoman",
        role: "user",
        enabled: false,
        deviceCount: 0,
        deviceLimit: 5,
        refreshEnabled: false,
        credential: "unbound",
        lastActive: "9 月 8 日",
        requests: 0,
        sessions: 0,
      },
      {
        id: "admin-1",
        username: "admin",
        role: "admin",
        enabled: true,
        deviceCount: 0,
        deviceLimit: 0,
        refreshEnabled: false,
        credential: "unbound",
        lastActive: "现在",
        requests: 0,
        sessions: 1,
      },
    ],
    incidents: [
      {
        id: "issue-1",
        userId: "user-2",
        title: "充电平台登录失效",
        detail: "自动恢复未成功，需要该用户重新扫码登录。最近一次恢复：18:35。",
        level: "critical",
        status: "open",
        occurrences: 3,
        time: "18:35",
        note: "",
      },
      {
        id: "issue-2",
        userId: "user-1",
        title: "充电桩持续离线",
        detail:
          "科创园 · B 座（608204）连续三次未返回端口状态，请检查网络或现场供电。",
        level: "warning",
        status: "open",
        occurrences: 3,
        time: "18:10",
        note: "",
      },
      {
        id: "issue-3",
        userId: "user-3",
        title: "微信投递等待重试",
        detail: "供应商暂时不可用，通知已保存在站内。尚未确认送达手机。",
        level: "warning",
        status: "acknowledged",
        occurrences: 1,
        time: "17:52",
        note: "等待下次自动重试。",
      },
    ],
    audit: [
      {
        id: "audit-1",
        action: "调整设备上限",
        target: "linyi · 10 台",
        time: "今天 17:30",
        result: "success",
      },
      {
        id: "audit-2",
        action: "确认异常",
        target: "微信投递等待重试",
        time: "今天 17:54",
        result: "success",
      },
      {
        id: "audit-3",
        action: "更新提醒策略",
        target: "检查间隔 5 分钟",
        time: "昨天 10:20",
        result: "success",
      },
    ],
    invites: [
      {
        id: "invite-1",
        code: "GARDEN-2026",
        enabled: true,
        uses: 2,
        expires: "2026-09-30",
      },
      {
        id: "invite-2",
        code: "SUMMER-2026",
        enabled: false,
        uses: 4,
        expires: "2026-08-31",
      },
    ],
    settings: {
      openRegistration: true,
      inviteRequired: true,
      defaultDeviceLimit: 10,
      defaultRefreshEnabled: true,
      statsRetentionDays: 30,
      portHistoryRetentionDays: 90,
      backgroundRemindersEnabled: true,
      recurringRemindersEnabled: false,
      watchRefreshIntervalMinutes: 5,
      watchPileLimitPerUser: 3,
      watchDailyRefreshQuota: 120,
      notificationRetentionDays: 30,
      scheduledPowerOffEnabled: true,
      scheduledPowerOffStartMinute: 1380,
      scheduledPowerOffEndMinute: 360,
      scheduledPowerOffTimezone: "Asia/Shanghai",
      powerRestoreJitterMinutes: 10,
    },
  }
}

export const statusLabel = {
  idle: "空闲",
  in_use: "使用中",
  offline: "离线",
} as const
export const credentialLabel: Record<CredentialState, string> = {
  unbound: "尚未连接",
  waiting_device: "等待添加充电桩",
  healthy: "连接正常",
  sync_failed: "同步失败",
  expired: "登录已失效",
}
export const durationLabel: Record<Duration, string> = {
  "1h": "1 小时",
  "2h": "2 小时",
  "4h": "4 小时",
  until_power_off: "到今晚断电前",
}
export const watchLabel: Record<DemoWatch["status"], string> = {
  active: "等待空闲",
  notified: "已提醒",
  expired: "已到期",
  cancelled: "已取消",
  legacy: "旧规则 · 已停用",
}
export const viewLabel: Record<View, string> = {
  piles: "常用充电桩",
  reminders: "空闲提醒",
  notifications: "通知",
  account: "账户与设置",
  login: "登录",
  register: "创建账户",
  admin: "运行概览",
  users: "用户",
  incidents: "异常处理",
  operations: "系统运行",
  policies: "系统策略",
  invites: "邀请注册",
  audit: "操作记录",
}
export const adminViews = new Set<View>([
  "admin",
  "users",
  "incidents",
  "operations",
  "policies",
  "invites",
  "audit",
])

export function counts(p: Pile) {
  return p.ports.reduce(
    (acc, port) => {
      acc[port.status]++
      return acc
    },
    { idle: 0, in_use: 0, offline: 0 }
  )
}
export function isActionNotice(n: DemoNotice) {
  return n.type === "pile_offline" || n.type === "credential_expired"
}
export function filterPiles(piles: Pile[], query: string, filter: string) {
  const q = query.trim().toLocaleLowerCase()
  return piles
    .filter((p) =>
      [p.name, p.address, p.number, p.id].some((v) =>
        v.toLocaleLowerCase().includes(q)
      )
    )
    .filter(
      (p) =>
        filter === "all" ||
        (filter === "idle"
          ? p.online && counts(p).idle > 0
          : filter === "busy"
            ? p.online && counts(p).idle === 0
            : !p.online && p.ports.length > 0)
    )
}
export function parseRoute(hash: string): Route {
  const [path, search] = hash.replace(/^#\/?/, "").split("?")
  const [rawView, id] = path.split("/")
  const view = Object.hasOwn(viewLabel, rawView) ? (rawView as View) : "piles"
  const params = new URLSearchParams(search)
  const port = Number(params.get("port"))
  return {
    view,
    id: id || undefined,
    tab: params.get("tab") || undefined,
    port: Number.isInteger(port) && port > 0 ? port : undefined,
  }
}
export function routeHref(route: Route): string {
  const params = new URLSearchParams()
  if (route.tab) params.set("tab", route.tab)
  if (route.port) params.set("port", String(route.port))
  return `#/${route.view}${route.id ? `/${route.id}` : ""}${params.size ? `?${params}` : ""}`
}
export function timeLabel(value: string, withDate = false) {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    ...(withDate ? ({ month: "numeric", day: "numeric" } as const) : {}),
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date)
}
export function validateIdentifier(value: string, existing: Pile[]) {
  const v = value.trim()
  if (!/^\d{6,64}$/.test(v)) return "请输入 6–64 位数字桩号或设备 ID。"
  if (existing.some((p) => p.id === v || p.number === v))
    return "这个充电桩已经在常用列表里了。"
  return ""
}
export function watchEnd(
  duration: Duration,
  now: string,
  powerOffEnabled: boolean,
  powerOffMinute = 1380
) {
  const start = new Date(now)
  const endOfDay = new Date(
    `${new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).format(start)}T00:00:00+08:00`
  )
  endOfDay.setTime(endOfDay.getTime() + powerOffMinute * 60_000)
  const hours = duration === "1h" ? 1 : duration === "4h" ? 4 : 2
  const end =
    duration === "until_power_off"
      ? endOfDay.getTime()
      : start.getTime() + hours * 3_600_000
  return new Date(
    powerOffEnabled ? Math.min(end, endOfDay.getTime()) : end
  ).toISOString()
}
export function safeData(serialized: string): PrototypeData {
  try {
    const parsed = JSON.parse(serialized) as PrototypeData
    if (
      parsed.version !== 2 ||
      !Array.isArray(parsed.piles) ||
      !Array.isArray(parsed.watches) ||
      !Array.isArray(parsed.notices) ||
      !Array.isArray(parsed.users) ||
      !Array.isArray(parsed.incidents) ||
      !Array.isArray(parsed.invites) ||
      !Array.isArray(parsed.audit) ||
      !parsed.settings ||
      !["user", "admin", "guest"].includes(parsed.demoRole) ||
      !["light", "dark", "system"].includes(parsed.theme) ||
      typeof parsed.username !== "string" ||
      !Number.isFinite(new Date(parsed.snapshotAt).getTime()) ||
      !parsed.piles.every(
        (p) =>
          typeof p.id === "string" &&
          typeof p.name === "string" &&
          typeof p.number === "string" &&
          typeof p.address === "string" &&
          Array.isArray(p.ports) &&
          p.ports.every(
            (port) =>
              Number.isInteger(port.id) &&
              ["idle", "in_use", "offline"].includes(port.status)
          )
      ) ||
      !parsed.watches.every(
        (w) =>
          typeof w.id === "string" &&
          typeof w.deviceId === "string" &&
          ["active", "notified", "expired", "cancelled", "legacy"].includes(
            w.status
          )
      ) ||
      !parsed.notices.every(
        (n) =>
          typeof n.title === "string" &&
          typeof n.message === "string" &&
          [
            "pile_available",
            "pile_offline",
            "pile_recovered",
            "credential_expired",
          ].includes(n.type)
      )
    )
      return initialData()
    return { ...initialData(), ...parsed }
  } catch {
    return initialData()
  }
}

// Deterministic, explicitly labeled sample observations, never a prediction.
export function historyBuckets(range: string, portId?: number) {
  const length = range === "24h" ? 24 : range === "30d" ? 30 : 7
  return Array.from({ length }, (_, i) => {
    const interval = range === "24h" ? 3_600_000 : 86_400_000
    const at = new Date(
      new Date(SNAPSHOT_AT).getTime() - (length - 1 - i) * interval
    )
    const observed = i === 0 ? 0.64 : 0.92
    const idle = [
      0.72, 0.64, 0.46, 0.32, 0.53, 0.78, 0.68, 0.55, 0.35, 0.23, 0.41, 0.61,
    ][(i + (portId ?? 0)) % 12]
    return {
      label:
        range === "24h"
          ? new Intl.DateTimeFormat("zh-CN", {
              timeZone: "Asia/Shanghai",
              hour: "2-digit",
              minute: "2-digit",
              hour12: false,
            }).format(at)
          : new Intl.DateTimeFormat("zh-CN", {
              timeZone: "Asia/Shanghai",
              month: "numeric",
              day: "numeric",
            }).format(at),
      at: at.toISOString(),
      idle,
      busy: observed - idle * observed,
      unknown: 1 - observed,
      observed,
    }
  })
}
