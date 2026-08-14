import { expect, test, type Page } from "@playwright/test"

import type {
  AdminTrendRange,
  AdminTrendsResponse,
  DeviceHistoryResponse,
  Notification,
  NotificationPreference,
  PortHistoryMetrics,
  PortHistoryResponse,
  PortStatus,
  WatchOverview,
  WatchRule,
} from "@/lib/api/generated"

function adminTrends(range: AdminTrendRange): AdminTrendsResponse {
  const count = range === "24h" ? 24 : range === "7d" ? 7 : 30
  const bucketUnit = range === "24h" ? "hour" : "day"
  const end = new Date("2026-08-05T10:00:00Z")
  const bucketMs = bucketUnit === "hour" ? 60 * 60 * 1000 : 24 * 60 * 60 * 1000
  const start = new Date(end.getTime() - count * bucketMs)
  const points = Array.from({ length: count }, (_, index) => {
    const pointStart = new Date(start.getTime() + index * bucketMs)
    const pointEnd = new Date(pointStart.getTime() + bucketMs)
    return {
      start: pointStart.toISOString(),
      end: pointEnd.toISOString(),
      requests: 20 + index,
      remoteAttempts: 10,
      remoteSuccesses: 9,
      remoteFailures: 1,
      remoteSuccessRate: 90,
      activeUsers: 2 + (index % 7),
      offlinePorts: index % 3,
    }
  })
  return {
    window: {
      range,
      timezone: "Asia/Shanghai",
      bucketUnit,
      start: start.toISOString(),
      end: end.toISOString(),
    },
    summary: {
      requests: points.reduce((total, point) => total + point.requests, 0),
      remoteAttempts: count * 10,
      remoteSuccesses: count * 9,
      remoteFailures: count,
      remoteSuccessRate: 90,
      activeUsers: 8,
      offlinePorts: points.at(-1)?.offlinePorts ?? 0,
    },
    points,
    updatedAt: end.toISOString(),
  }
}

const user = {
  id: "user-wang",
  username: "wang",
  role: "user",
  enabled: true,
  createdAt: "2026-07-15T00:00:00Z",
  deviceLimit: 10,
  refreshEnabled: true,
  usageGuideAckAt: "2026-07-15T00:00:00Z",
}

const ports = Array.from({ length: 10 }, (_, index) => ({
  id: index + 1,
  status: (index === 2
    ? "idle"
    : index === 4
      ? "offline"
      : "in_use") as PortStatus,
  powerKw: index === 2 ? 0 : 6.6,
  energyKwh: index === 2 ? 0 : 2.4,
  updatedAt: "2026-07-15T09:00:00Z",
  sessionMin: index === 2 ? 0 : 35,
  usedSeconds: index === 2 ? 0 : 2100,
  usedText: "35 分钟",
  remainingText: "25 分钟",
}))

const snapshot = {
  piles: [
    {
      id: "2601201412385560001",
      number: "61034278",
      name: "松园 3 号楼",
      status: "在线",
      address: "北门",
      openNum: 10,
      online: true,
      createdAt: "2026-07-15T00:00:00Z",
      updatedAt: "2026-07-15T09:00:00Z",
      source: "remote",
      ports,
      usedPortIds: ports
        .filter((port) => port.status === "in_use")
        .map((port) => port.id),
      sortOrder: 0,
    },
  ],
  updatedAt: "2026-07-15T09:00:00Z",
  statistics: {
    pileCount: 1,
    portCount: 10,
    inUsePortCount: 8,
    idlePortCount: 1,
    offlinePorts: 1,
  },
  refresh: {
    minIntervalSeconds: 30,
    attemptedDevices: 1,
    successfulDevices: 1,
    failedDevices: 0,
    skippedDevices: 0,
    cached: false,
    partial: false,
    message: "已更新 1 台充电桩",
    lastRemoteAt: "2026-07-15T09:00:00Z",
  },
}

async function mockEmptyWatchResources(page: Page) {
  await page.route("**/api/watch-rules", (route) =>
    route.fulfill({ status: 200, json: [] })
  )
  await page.route("**/api/watch-overview", (route) =>
    route.fulfill({
      status: 200,
      json: {
        reminderPileCount: 0,
        reminderPileLimit: 5,
        dailyQuotaUsed: 0,
        dailyQuotaLimit: 480,
        quotaDate: "2026-08-10",
        refreshIntervalMinutes: 10,
        backgroundRemindersEnabled: true,
        accountRefreshEnabled: true,
        scheduledPowerOffEnabled: true,
        scheduledPowerOffStartMinute: 1380,
        scheduledPowerOffEndMinute: 420,
        scheduledPowerOffTimezone: "Asia/Shanghai",
      },
    })
  )
  await page.route("**/api/notification-preferences", (route) =>
    route.fulfill({
      status: 200,
      json: {
        userId: user.id,
        browserEnabled: false,
        quietHoursEnabled: true,
        quietStartMinute: 1320,
        quietEndMinute: 480,
        timezone: "Asia/Shanghai",
        updatedAt: "2026-08-10T00:00:00Z",
      },
    })
  )
  await page.route("**/api/notifications**", (route) =>
    route.fulfill({ status: 200, json: { items: [], unreadCount: 0 } })
  )
}

const historyMetrics: PortHistoryMetrics = {
  observedSeconds: 432000,
  gapSeconds: 172800,
  idleSeconds: 302400,
  inUseSeconds: 129600,
  offlineSeconds: 0,
  occupancyPercent: 30,
  completedSessions: 6,
  averageSessionSeconds: 3600,
  sampleState: "partial",
}

const historyDaily = Array.from({ length: 7 }, (_, index) => ({
  date: `2026-07-${String(9 + index).padStart(2, "0")}`,
  metrics: { ...historyMetrics, occupancyPercent: 20 + index * 5 },
}))

const deviceHistory: DeviceHistoryResponse = {
  device: {
    id: snapshot.piles[0].id,
    number: snapshot.piles[0].number,
    name: snapshot.piles[0].name,
    address: snapshot.piles[0].address,
  },
  window: {
    range: "7d",
    timezone: "Asia/Shanghai",
    start: "2026-07-09T09:00:00Z",
    end: "2026-07-16T09:00:00Z",
  },
  metrics: historyMetrics,
  daily: historyDaily,
  heatmap: Array.from({ length: 168 }, (_, index) => ({
    weekday: Math.floor(index / 24) + 1,
    hour: index % 24,
    idleSeconds: 7200,
    inUseSeconds: 3600,
    offlineSeconds: 0,
    occupancyPercent: 33.3,
    sampleDates: 4,
    sampleSufficient: true,
  })),
  ports: ports.map((port) => ({
    portId: port.id,
    currentStatus: port.status,
    metrics: historyMetrics,
  })),
  busiestHours: [{ weekday: 1, hour: 9, occupancyPercent: 78, sampleDates: 4 }],
  quietSuggestion: {
    weekday: 3,
    hour: 14,
    occupancyPercent: 12,
    sampleDates: 4,
  },
  historyStartedAt: "2026-07-09T09:00:00Z",
  historyNotice: "历史从启用记录后开始，首次记录之前的时段不会计为空闲。",
}

function portHistory(portId: number): PortHistoryResponse {
  return {
    device: deviceHistory.device,
    portId,
    currentStatus: ports[portId - 1].status,
    window: deviceHistory.window,
    metrics: historyMetrics,
    daily: historyDaily,
    timeline: [
      {
        portId,
        fromStatus: "idle",
        toStatus: "in_use",
        changedAt: "2026-07-15T08:00:00Z",
        usedSeconds: 0,
      },
      {
        portId,
        fromStatus: "in_use",
        toStatus: ports[portId - 1].status,
        changedAt: "2026-07-15T09:00:00Z",
        usedSeconds: 3600,
        remainingText: "25 分钟",
      },
    ],
    timelineTruncated: false,
    historyStartedAt: "2026-07-09T09:00:00Z",
    historyNotice: deviceHistory.historyNotice,
  }
}

test("administrator handles a user and verifies the audit trail", async ({
  page,
}) => {
  await page.goto("/login")
  await page.getByLabel("用户名").fill("admin")
  await page.locator("#password").fill("localadmin123")
  await page.getByRole("button", { name: "登录", exact: true }).click()

  await expect(page).toHaveURL(/\/admin\/?$/)
  await expect(page.getByRole("heading", { name: "运营总览" })).toBeVisible()
  await page.getByRole("button", { name: "创建用户" }).click()
  await page.locator("#new-username").fill("e2e-user")
  await page.locator("#new-password").fill("password123")
  await page.getByRole("button", { name: "确认创建" }).click()
  await expect(page.getByText("用户已创建")).toBeVisible()

  await page.getByRole("tab", { name: "用户管理" }).click()
  await expect(page).toHaveURL(/\/admin\/?\?tab=users$/)
  await expect(page.getByRole("heading", { name: "筛选用户" })).toBeVisible()
  await page.getByPlaceholder("用户名").fill("e2e-user")
  await page.getByRole("button", { name: "应用筛选" }).click()
  await expect(
    page.getByLabel("用户目录").getByText("e2e-user", { exact: true })
  ).toBeVisible()
  await page
    .getByLabel("用户目录")
    .getByRole("button", { name: "管理用户 e2e-user" })
    .click()
  await expect(page.getByRole("heading", { name: /e2e-user/ })).toBeVisible()
  await expect(page.getByText("当前登录会话")).toBeVisible()
  await page.getByRole("button", { name: "停用账户" }).click()
  await expect(page.getByText("用户已停用")).toBeVisible()
  await page.getByRole("button", { name: "启用账户" }).click()
  await expect(page.getByText("用户已启用")).toBeVisible()
  await page.getByRole("button", { name: "重置密码" }).click()
  await page.getByRole("button", { name: "生成临时密码" }).click()
  await expect(
    page.getByRole("heading", { name: "临时密码已生成" })
  ).toBeVisible()
  await expect(page.locator("code")).not.toBeEmpty()
  await page.getByRole("button", { name: "我已保存" }).click()
  await page
    .locator("[data-slot=sheet-content]")
    .getByRole("button", { name: "关闭" })
    .click()

  await page.screenshot({
    path: "/tmp/charge-1.4.13-admin-users.png",
    fullPage: false,
  })

  await page.getByRole("tab", { name: "系统设置" }).click()
  await page.getByLabel("端口历史保留天数").fill("120")
  await page.getByRole("button", { name: "保存设置" }).click()
  await expect(page.getByText("系统设置已保存")).toBeVisible()
  await expect(
    page.getByText("空闲提醒与后台调度", { exact: true })
  ).toBeVisible()
  await page.getByLabel("刷新间隔（分钟）").fill("12")
  await page.getByRole("button", { name: "保存提醒策略" }).click()
  await expect(page.getByText("系统设置已保存")).toBeVisible()
  await page.getByRole("tab", { name: "运维审计" }).click()
  await expect(page.getByText(/后台按整桩低频刷新/)).toBeVisible()
  await expect(page.getByText("空闲提醒充电桩")).toBeVisible()
  await expect(page.getByText("24 小时请求成功率")).toBeVisible()
  await expect(page.getByText("数据与备份")).toBeVisible()
  await expect(page.getByText("端口历史")).toBeVisible()
  await expect(page.getByText("站内通知")).toBeVisible()
  await expect(page.getByText(/保留 120 天/)).toBeVisible()
  await page.getByRole("button", { name: "重新检查" }).click()
  await expect(page.getByText("运维状态已重新检查")).toBeVisible()
  await expect(page.getByText("管理操作日志")).toBeVisible()
  await expect(page.getByText(/修改账户状态/).first()).toBeVisible()
  await expect(page.getByText(/重置密码/).first()).toBeVisible()
  await expect(page.getByText(/修改系统设置/).first()).toBeVisible()
  await page.screenshot({
    path: "/tmp/charge-1.5.1-admin-operations.png",
    fullPage: false,
  })

  await page.setViewportSize({ width: 375, height: 812 })
  await expect(page.getByText("端口历史")).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  await page.screenshot({
    path: "/tmp/charge-1.5.1-operations-mobile.png",
    fullPage: false,
  })
  await page.getByRole("tab", { name: "运营总览" }).click()
  await expect(page.getByRole("tab", { name: "运营总览" })).toHaveAttribute(
    "data-active",
    ""
  )
  await expect(page.getByRole("tab", { name: "运维审计" })).not.toHaveAttribute(
    "data-active"
  )
  await expect(page.getByRole("heading", { name: "运营总览" })).toBeVisible()
  const issueMetric = await page.getByLabel("待处理问题 指标").boundingBox()
  const successMetric = await page.getByLabel("远端成功率 指标").boundingBox()
  const activeMetric = await page.getByLabel("活跃用户 指标").boundingBox()
  expect(successMetric?.y).toBeGreaterThan(issueMetric?.y ?? 0)
  expect(successMetric?.y).toBe(activeMetric?.y)

  await page.screenshot({
    path: "/tmp/charge-1.4.13-admin-mobile.png",
    fullPage: false,
  })

  await page.getByRole("button", { name: "打开菜单" }).click()
  const mobileMenu = page.locator("[data-slot=sheet-content]")
  await expect(mobileMenu.getByText("当前页面")).toBeVisible()
  await expect(mobileMenu.getByText("账户操作")).toBeVisible()
  await expect(
    mobileMenu.getByRole("button", { name: "创建用户" })
  ).toBeVisible()
  await expect(
    mobileMenu.getByRole("button", { name: "刷新数据" })
  ).toBeVisible()
  await page.waitForTimeout(250)
  const mobileMenuBox = await mobileMenu.boundingBox()
  expect(mobileMenuBox?.x).toBeGreaterThanOrEqual(0)
  expect(
    (mobileMenuBox?.x ?? 0) + (mobileMenuBox?.width ?? 0)
  ).toBeLessThanOrEqual(376)
  const refreshBox = await mobileMenu
    .getByRole("button", { name: "刷新数据" })
    .boundingBox()
  const logoutBox = await mobileMenu
    .getByRole("button", { name: "退出登录" })
    .boundingBox()
  expect(logoutBox?.y).toBeGreaterThan(
    (refreshBox?.y ?? 0) + (refreshBox?.height ?? 0) + 16
  )
  await page.screenshot({
    path: "/tmp/charge-1.4.13-admin-mobile-menu.png",
    fullPage: false,
  })
})

test("administrator switches trend ranges, reads tooltips, and exports matching CSV", async ({
  page,
}) => {
  await page.route("**/api/admin/trends?*", async (route) => {
    const range = (new URL(route.request().url()).searchParams.get("range") ??
      "24h") as AdminTrendRange
    await route.fulfill({ json: adminTrends(range) })
  })
  await page.route("**/api/admin/trends.csv?*", async (route) => {
    const range =
      new URL(route.request().url()).searchParams.get("range") ?? "24h"
    await route.fulfill({
      status: 200,
      contentType: "text/csv; charset=utf-8",
      headers: {
        "Content-Disposition": `attachment; filename="charge-trends-${range}-2026-08-05.csv"`,
      },
      body: "\uFEFF时间段开始,时间段结束,请求量\n2026-08-04T10:00:00Z,2026-08-05T10:00:00Z,120\n",
    })
  })

  await page.goto("/login")
  await page.getByLabel("用户名").fill("admin")
  await page.locator("#password").fill("localadmin123")
  await page.getByRole("button", { name: "登录", exact: true }).click()

  await expect(
    page.getByRole("group", { name: "24 小时远端成功率趋势" })
  ).toBeVisible()
  await page.getByRole("tab", { name: "7 天", exact: true }).click()
  await expect(page).toHaveURL(/\/admin\?range=7d$/)
  const chart = page.getByRole("group", { name: "7 天远端成功率趋势" })
  await expect(chart).toBeVisible()

  await page.getByRole("tab", { name: "活跃用户", exact: true }).click()
  await expect(page.getByText("8 人", { exact: true })).toBeVisible()
  const activeChart = page.getByRole("group", { name: "7 天活跃用户趋势" })
  await activeChart.getByRole("button").first().focus()
  await expect(page.getByText(/活跃用户 2 人/)).toBeVisible()

  await page.reload()
  await expect(page).toHaveURL(/\/admin\?range=7d$/)
  await expect(page.getByRole("tab", { name: "7 天" })).toHaveAttribute(
    "aria-selected",
    "true"
  )
  await expect(
    page.getByRole("group", { name: "7 天远端成功率趋势" })
  ).toBeVisible()

  const downloadPromise = page.waitForEvent("download")
  await page.getByRole("button", { name: "导出 CSV" }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toBe("charge-trends-7d-2026-08-05.csv")
  await expect(page.getByText("7 天趋势已导出")).toBeVisible()

  await page.screenshot({
    path: "/tmp/charge-1.5.0-admin-trends.png",
    fullPage: false,
  })
})

test("dashboard restores URL filters and keeps the mobile controls clear", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.addInitScript(() => {
    const animationWindow = window as typeof window & {
      __pileCardAnimations?: string[]
    }
    animationWindow.__pileCardAnimations = []
    document.addEventListener("animationstart", (event) => {
      if (
        event.target instanceof HTMLElement &&
        event.target.classList.contains("pile-card-enter")
      ) {
        animationWindow.__pileCardAnimations?.push(event.animationName)
      }
    })

    class LocalEventSource {
      static OPEN = 1
      readyState = LocalEventSource.OPEN
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor() {
        setTimeout(() => this.onopen?.(new Event("open")), 0)
      }

      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, "EventSource", {
      configurable: true,
      value: LocalEventSource,
    })
  })
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 200, json: user })
  )
  await page.route("**/api/piles", (route) =>
    route.fulfill({ status: 200, json: snapshot })
  )
  await mockEmptyWatchResources(page)

  await page.goto("/dashboard?q=03&status=idle")

  const search = page.getByPlaceholder("名称、桩号、地址或端口号")
  await expect(search).toHaveValue("03")
  await expect(search).toHaveCSS("padding-left", "40px")
  await expect(page.getByText("03", { exact: true })).toBeVisible()
  await expect(page.getByText("等待使用")).toBeVisible()
  await expect(page.getByText("设备暂不可访问")).toHaveCount(0)
  await expect(
    page.locator('[aria-label="充电桩列表"] [style*="animation-delay"]')
  ).toHaveCount(0)
  await expect(
    page.locator('section[aria-label="充电桩列表"] > div').first()
  ).not.toHaveClass(/slide-in-from-bottom/)
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as typeof window & {
              __pileCardAnimations?: string[]
            }
          ).__pileCardAnimations ?? []
      )
    )
    .toEqual(["pile-card-enter"])

  const firstMetric = await page.getByLabel("充电桩 指标").boundingBox()
  const secondMetric = await page.getByLabel("全部端口 指标").boundingBox()
  const thirdMetric = await page.getByLabel("正在使用 指标").boundingBox()
  expect(firstMetric?.y).toBe(secondMetric?.y)
  expect(thirdMetric?.y).toBeGreaterThan(firstMetric?.y ?? 0)

  await search.fill("松园")
  await expect
    .poll(() => new URL(page.url()).searchParams.get("q"))
    .toBe("松园")
  await expect(page.getByText("03", { exact: true })).toBeVisible()
  await expect(
    page.evaluate(
      () =>
        (
          window as typeof window & {
            __pileCardAnimations?: string[]
          }
        ).__pileCardAnimations ?? []
    )
  ).resolves.toEqual(["pile-card-enter"])

  await page.screenshot({
    path: "/tmp/charge-1.4.13-dashboard-mobile.png",
    fullPage: false,
  })
})

test("dashboard fades in every port card at the same time", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.addInitScript(() => {
    const motionWindow = window as typeof window & {
      __pileCardStarts?: number[]
      __portCardStarts?: number[]
    }
    motionWindow.__pileCardStarts = []
    motionWindow.__portCardStarts = []
    document.addEventListener("animationstart", (event) => {
      if (!(event.target instanceof HTMLElement)) return
      if (event.target.classList.contains("pile-card-enter")) {
        motionWindow.__pileCardStarts?.push(performance.now())
      }
      if (event.target.classList.contains("port-card-enter")) {
        motionWindow.__portCardStarts?.push(performance.now())
      }
    })

    class LocalEventSource {
      static OPEN = 1
      readyState = LocalEventSource.OPEN
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor() {
        setTimeout(() => this.onopen?.(new Event("open")), 0)
      }

      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, "EventSource", {
      configurable: true,
      value: LocalEventSource,
    })
  })
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 200, json: user })
  )
  await page.route("**/api/piles", (route) =>
    route.fulfill({ status: 200, json: snapshot })
  )
  await mockEmptyWatchResources(page)

  await page.goto("/dashboard")
  await expect(page.locator('[aria-label$="号充电口"]')).toHaveCount(10)

  await expect
    .poll(() =>
      page.evaluate(() => {
        const motionWindow = window as typeof window & {
          __pileCardStarts?: number[]
          __portCardStarts?: number[]
        }
        return {
          piles: motionWindow.__pileCardStarts ?? [],
          ports: motionWindow.__portCardStarts ?? [],
        }
      })
    )
    .toMatchObject({
      piles: [expect.any(Number)],
      ports: Array.from({ length: 10 }, () => expect.any(Number)),
    })

  const motionStarts = await page.evaluate(() => {
    const motionWindow = window as typeof window & {
      __pileCardStarts?: number[]
      __portCardStarts?: number[]
    }
    return {
      piles: motionWindow.__pileCardStarts ?? [],
      ports: motionWindow.__portCardStarts ?? [],
    }
  })
  const portStarts = motionStarts.ports
  expect(Math.max(...portStarts) - Math.min(...portStarts)).toBeLessThan(20)
  await expect(
    page.locator('[aria-label$="号充电口"][style*="animation-delay"]')
  ).toHaveCount(0)
  await expect(page.locator(".port-card-enter")).toHaveCount(0)

  await page.screenshot({
    path: "/tmp/charge-1.4.13-dashboard-port-motion.png",
    fullPage: false,
  })
})

test("dashboard creates and manages whole-pile cross-midnight reminders", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.addInitScript(() => {
    class LocalEventSource {
      static OPEN = 1
      readyState = LocalEventSource.OPEN
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor() {
        setTimeout(() => this.onopen?.(new Event("open")), 0)
      }

      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, "EventSource", {
      configurable: true,
      value: LocalEventSource,
    })
  })

  const rules: WatchRule[] = []
  const overview: WatchOverview = {
    reminderPileCount: 0,
    reminderPileLimit: 5,
    dailyQuotaUsed: 7,
    dailyQuotaLimit: 480,
    quotaDate: "2026-08-10",
    refreshIntervalMinutes: 10,
    backgroundRemindersEnabled: true,
    accountRefreshEnabled: true,
    scheduledPowerOffEnabled: true,
    scheduledPowerOffStartMinute: 1380,
    scheduledPowerOffEndMinute: 420,
    scheduledPowerOffTimezone: "Asia/Shanghai",
  }
  const preference: NotificationPreference = {
    userId: user.id,
    browserEnabled: false,
    quietHoursEnabled: true,
    quietStartMinute: 1320,
    quietEndMinute: 480,
    timezone: "Asia/Shanghai",
    updatedAt: "2026-08-10T00:00:00Z",
  }

  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 200, json: user })
  )
  await page.route("**/api/piles", (route) =>
    route.fulfill({ status: 200, json: snapshot })
  )
  await page.route("**/api/watch-overview", (route) =>
    route.fulfill({ status: 200, json: overview })
  )
  await page.route("**/api/notification-preferences", async (route) => {
    if (route.request().method() === "PATCH") {
      Object.assign(preference, route.request().postDataJSON())
      preference.updatedAt = "2026-08-10T00:01:00Z"
    }
    await route.fulfill({ status: 200, json: preference })
  })
  await page.route("**/api/notifications**", (route) =>
    route.fulfill({ status: 200, json: { items: [], unreadCount: 0 } })
  )
  await page.route("**/api/watch-rules", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, json: rules })
      return
    }
    const payload = route.request().postDataJSON()
    const created: WatchRule = {
      id: `rule-${rules.length + 1}`,
      userId: user.id,
      deviceId: payload.deviceId,
      enabled: payload.enabled ?? true,
      activeWeekdays: payload.activeWeekdays ?? 127,
      activeStartMinute: payload.activeStartMinute ?? 0,
      activeEndMinute: payload.activeEndMinute ?? 0,
      timezone: payload.timezone ?? "Asia/Shanghai",
      createdAt: "2026-08-10T00:00:00Z",
      updatedAt: "2026-08-10T00:00:00Z",
    }
    rules.push(created)
    await route.fulfill({ status: 201, json: created })
  })

  await page.goto("/dashboard")
  await page.getByRole("button", { name: "设置空闲提醒" }).click()
  await page.getByLabel("开始时间").fill("22:30")
  await page.getByLabel("结束时间").fill("06:30")
  await expect(page.getByText(/22:30–06:30（跨午夜）/)).toBeVisible()
  await page.getByRole("button", { name: "创建提醒" }).click()
  await expect(
    page.getByRole("button", { name: "已设置空闲提醒" })
  ).toBeVisible()

  await page.getByRole("button", { name: "空闲提醒", exact: true }).click()
  const sheet = page.getByRole("dialog", { name: "空闲提醒管理" })
  await expect(sheet.getByText("1/5")).toBeVisible()
  await expect(sheet.getByText("每天 · 22:30–06:30（跨午夜）")).toBeVisible()
  await expect(sheet.getByText("7/480")).toBeVisible()

  await sheet.getByRole("button", { name: "添加提醒" }).click()
  const editor = page.getByRole("dialog", { name: "添加空闲提醒" })
  await expect(editor).toBeVisible()
  await expect(sheet).toBeHidden()
  await editor.getByRole("button", { name: "关闭" }).click()
  await page.setViewportSize({ width: 375, height: 812 })
  await page.getByRole("button", { name: "打开菜单" }).click()
  await page
    .locator("[data-slot=sheet-content]")
    .getByRole("button", { name: "空闲提醒", exact: true })
    .click()
  await expect(sheet.getByText("空闲提醒管理")).toBeVisible()
  await page.waitForTimeout(250)
  const sheetBox = await sheet.boundingBox()
  expect(sheetBox?.x).toBeGreaterThanOrEqual(0)
  expect((sheetBox?.x ?? 0) + (sheetBox?.width ?? 0)).toBeLessThanOrEqual(376)
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth)
  ).toBeLessThanOrEqual(375)
  await page.screenshot({
    path: "/tmp/charge-1.5.1-watch-mobile.png",
    fullPage: false,
  })
})

test("dashboard history sheet supports port navigation and mobile layout", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.addInitScript(() => {
    class LocalEventSource {
      static OPEN = 1
      readyState = LocalEventSource.OPEN
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor() {
        setTimeout(() => this.onopen?.(new Event("open")), 0)
      }

      addEventListener() {}
      close() {}
    }
    Object.defineProperty(window, "EventSource", {
      configurable: true,
      value: LocalEventSource,
    })
  })
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 200, json: user })
  )
  await page.route("**/api/piles", (route) =>
    route.fulfill({ status: 200, json: snapshot })
  )
  await mockEmptyWatchResources(page)
  await page.route("**/api/piles/*/ports/*/history**", (route) => {
    const pathname = new URL(route.request().url()).pathname
    const portId = Number(pathname.match(/ports\/(\d+)\/history/)?.[1] ?? 1)
    return route.fulfill({ status: 200, json: portHistory(portId) })
  })
  await page.route("**/api/piles/*/history**", (route) =>
    route.fulfill({ status: 200, json: deviceHistory })
  )

  await page.goto("/dashboard")
  await page.getByRole("button", { name: "历史趋势" }).click()

  const sheet = page.locator("[data-slot=sheet-content]")
  await expect(
    sheet.getByRole("heading", { name: /松园 3 号楼/ })
  ).toBeVisible()
  await expect(sheet.getByText("最近 7 天占用率")).toBeVisible()
  await expect(
    sheet.getByRole("group", { name: "星期与小时占用率热力图" })
  ).toBeVisible()
  await expect(
    sheet.getByRole("list", { name: "1 号口状态时间线" })
  ).toBeVisible()

  await page.screenshot({
    path: "/tmp/charge-1.5.0-history-desktop.png",
    fullPage: false,
  })

  const firstPort = sheet.getByRole("tab", { name: "01 号" })
  await firstPort.focus()
  await page.keyboard.press("ArrowRight")
  await expect(sheet.getByRole("tab", { name: "02 号" })).toBeFocused()
  await expect(
    sheet.getByRole("list", { name: "2 号口状态时间线" })
  ).toBeVisible()

  await page.setViewportSize({ width: 375, height: 812 })
  const mobileBox = await sheet.boundingBox()
  expect(mobileBox?.x).toBeGreaterThanOrEqual(0)
  expect((mobileBox?.x ?? 0) + (mobileBox?.width ?? 0)).toBeLessThanOrEqual(376)
  await sheet
    .locator('[data-slot="history-scroll"]')
    .evaluate((element) => element.scrollTo({ top: 0 }))
  await expect(sheet.getByText("使用概览")).toBeVisible()
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth)
  ).toBeLessThanOrEqual(375)
  await page.screenshot({
    path: "/tmp/charge-1.5.0-history-mobile.png",
    fullPage: false,
  })
})

test("notification center closes the durable and browser alert loop on mobile", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.addInitScript(() => {
    class LocalEventSource {
      static OPEN = 1
      readyState = LocalEventSource.OPEN
      onopen: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null
      listeners = new Map<
        string,
        Array<(event: MessageEvent<string>) => void>
      >()

      constructor() {
        ;(
          window as typeof window & { __notificationStream?: LocalEventSource }
        ).__notificationStream = this
        setTimeout(() => this.onopen?.(new Event("open")), 0)
      }

      addEventListener(
        name: string,
        listener: (event: MessageEvent<string>) => void
      ) {
        const listeners = this.listeners.get(name) ?? []
        listeners.push(listener)
        this.listeners.set(name, listeners)
      }

      emit(name: string, value: unknown) {
        const event = new MessageEvent(name, { data: JSON.stringify(value) })
        this.listeners.get(name)?.forEach((listener) => listener(event))
      }

      close() {}
    }

    class LocalNotification {
      static permission: NotificationPermission = "default"
      static requestPermission = async () => {
        LocalNotification.permission = "granted"
        return "granted" as const
      }
      onclick: (() => void) | null = null

      constructor(title: string, options?: NotificationOptions) {
        ;(
          window as typeof window & {
            __browserNotifications?: Array<{
              title: string
              options?: NotificationOptions
            }>
          }
        ).__browserNotifications?.push({ title, options })
      }

      close() {}
    }

    ;(
      window as typeof window & {
        __browserNotifications?: Array<{
          title: string
          options?: NotificationOptions
        }>
      }
    ).__browserNotifications = []
    Object.defineProperty(window, "EventSource", {
      configurable: true,
      value: LocalEventSource,
    })
    Object.defineProperty(window, "Notification", {
      configurable: true,
      value: LocalNotification,
    })
  })

  const notifications: Notification[] = [
    {
      id: "notice-idle",
      userId: user.id,
      type: "pile_available",
      severity: "info",
      title: "松园 3 号楼有空闲口",
      message: "整桩从无空闲变为至少一个空闲口。",
      deviceId: snapshot.piles[0].id,
      portId: 3,
      createdAt: "2026-08-11T09:00:00Z",
    },
    {
      id: "notice-credential",
      userId: user.id,
      type: "credential_expired",
      severity: "critical",
      title: "扫码凭据已失效",
      message: "请重新扫码登录后恢复后台提醒。",
      createdAt: "2026-08-11T08:00:00Z",
    },
  ]
  const preference: NotificationPreference = {
    userId: user.id,
    browserEnabled: false,
    quietHoursEnabled: false,
    quietStartMinute: 1320,
    quietEndMinute: 480,
    timezone: "Asia/Shanghai",
    updatedAt: "2026-08-11T00:00:00Z",
  }

  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 200, json: user })
  )
  await page.route("**/api/piles", (route) =>
    route.fulfill({ status: 200, json: snapshot })
  )
  await page.route("**/api/watch-rules", (route) =>
    route.fulfill({ status: 200, json: [] })
  )
  await page.route("**/api/watch-overview", (route) =>
    route.fulfill({
      status: 200,
      json: {
        reminderPileCount: 0,
        reminderPileLimit: 5,
        dailyQuotaUsed: 0,
        dailyQuotaLimit: 480,
        quotaDate: "2026-08-11",
        refreshIntervalMinutes: 10,
        backgroundRemindersEnabled: true,
        accountRefreshEnabled: true,
        scheduledPowerOffEnabled: true,
        scheduledPowerOffStartMinute: 1380,
        scheduledPowerOffEndMinute: 420,
        scheduledPowerOffTimezone: "Asia/Shanghai",
      },
    })
  )
  await page.route("**/api/notification-preferences", async (route) => {
    if (route.request().method() === "PATCH") {
      Object.assign(preference, route.request().postDataJSON())
      preference.updatedAt = "2026-08-11T09:01:00Z"
    }
    await route.fulfill({ status: 200, json: preference })
  })
  await page.route("**/api/notifications**", async (route) => {
    const url = new URL(route.request().url())
    if (route.request().method() === "GET") {
      const status = url.searchParams.get("status") ?? "all"
      const items = notifications.filter((notification) =>
        status === "unread"
          ? !notification.readAt
          : status === "resolved"
            ? Boolean(notification.resolvedAt)
            : true
      )
      await route.fulfill({
        status: 200,
        json: {
          items,
          unreadCount: notifications.filter(
            (notification) => !notification.readAt
          ).length,
        },
      })
      return
    }
    const readMatch = url.pathname.match(/\/api\/notifications\/([^/]+)\/read$/)
    if (readMatch) {
      const notification = notifications.find(
        (candidate) => candidate.id === decodeURIComponent(readMatch[1])
      )
      if (notification) notification.readAt = "2026-08-11T09:02:00Z"
      await route.fulfill({ status: 200, json: notification })
      return
    }
    await route.fulfill({ status: 200, json: { updated: 0, deleted: 0 } })
  })
  await page.route("**/api/session/yyb-binding", (route) =>
    route.fulfill({ status: 200, json: { bound: false } })
  )

  await page.goto("/dashboard")
  const trigger = page.getByRole("button", { name: "通知，2 条未读" })
  await expect(trigger).toBeVisible()
  await trigger.click()
  const sheet = page.getByRole("dialog", { name: "通知中心" })
  await expect(sheet).toBeVisible()
  await sheet.getByRole("button", { name: "允许通知" }).click()
  await expect(
    sheet.getByRole("switch", { name: "浏览器即时提醒" })
  ).toBeChecked()

  await sheet.getByText("松园 3 号楼有空闲口").click()
  await expect(page.getByLabel("3 号充电口")).toHaveClass(
    /notification-target-glow/
  )

  await page.getByRole("button", { name: "通知，1 条未读" }).click()
  await page.getByText("扫码凭据已失效").click()
  await expect(
    page.getByRole("dialog", { name: "扫码登录远端账号" })
  ).toBeVisible()
  await page
    .getByRole("dialog", { name: "扫码登录远端账号" })
    .getByRole("button", { name: "关闭", exact: true })
    .first()
    .click()

  const incoming: Notification = {
    id: "notice-offline",
    userId: user.id,
    type: "pile_offline",
    severity: "warning",
    title: "松园 3 号楼持续离线",
    message: "非计划断电时段已连续检查失败。",
    deviceId: snapshot.piles[0].id,
    createdAt: "2026-08-11T09:03:00Z",
  }
  await page.evaluate((notification) => {
    ;(
      window as typeof window & {
        __notificationStream?: { emit: (name: string, value: unknown) => void }
      }
    ).__notificationStream?.emit("notification", notification)
  }, incoming)

  await expect(
    page.getByRole("button", { name: "通知，1 条未读" })
  ).toBeVisible()
  expect(
    await page.evaluate(
      () =>
        (
          window as typeof window & {
            __browserNotifications?: unknown[]
          }
        ).__browserNotifications?.length
    )
  ).toBe(1)
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth)
  ).toBeLessThanOrEqual(375)
})
