import { expect, test, type Page } from "@playwright/test"
import type { DashboardSnapshot } from "../lib/types"

function snapshot(): DashboardSnapshot {
  return {
    piles: [
      {
        id: "2601201412385560001",
        number: "61034278",
        name: "北门车棚",
        address: "北门入口右侧",
        status: "在线",
        online: true,
        openNum: 1,
        source: "remote",
        usedPortIds: [],
        sortOrder: 0,
        createdAt: "2026-09-11T01:00:00Z",
        updatedAt: "2026-09-11T02:00:00Z",
        ports: [
          {
            id: 1,
            status: "idle",
            powerKw: 0,
            energyKwh: 0,
            sessionMin: 0,
            usedSeconds: 0,
            updatedAt: "2026-09-11T02:00:00Z",
          },
        ],
      },
    ],
    updatedAt: "2026-09-11T02:00:00Z",
    statistics: {
      pileCount: 1,
      portCount: 1,
      idlePortCount: 1,
      inUsePortCount: 0,
      offlinePorts: 0,
    },
    refresh: {
      minIntervalSeconds: 30,
      attemptedDevices: 1,
      successfulDevices: 1,
      failedDevices: 0,
      skippedDevices: 0,
      cached: false,
      partial: false,
    },
  }
}

async function mockApi(page: Page, data: DashboardSnapshot, failFirst = false) {
  let failed = false
  await page.addInitScript(() => {
    // Keep the stream open without contacting a live service.
    window.EventSource = class extends EventTarget {
      onopen: (() => void) | null = null
      close() {}
      constructor() {
        super()
        setTimeout(() => this.onopen?.(), 0)
      }
    } as unknown as typeof EventSource
  })
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === "/api/piles") {
      if (failFirst && !failed) {
        failed = true
        return route.fulfill({
          status: 503,
          json: { error: "暂时无法读取充电桩" },
        })
      }
      return route.fulfill({ json: data })
    }
    if (path === "/api/auth/me")
      return route.fulfill({
        json: {
          id: "test-user",
          username: "预览用户",
          role: "user",
          enabled: true,
          refreshEnabled: true,
          deviceLimit: 10,
          createdAt: data.updatedAt,
          usageGuideAckAt: data.updatedAt,
        },
      })
    if (path === "/api/announcements/summary") return route.fulfill({ json: {item:null,unreadCount:0,serverNow:new Date().toISOString(),nextBoundary:null} })
    if (path === "/api/notification-preferences") return route.fulfill({ json: {browserEnabled:false,quietHoursEnabled:false,quietStartMinute:0,quietEndMinute:0,timezone:"Asia/Shanghai"} })
    if (path === "/api/watch-rules") return route.fulfill({ json: [] })
    if (path === "/api/watch-overview")
      return route.fulfill({
        json: {
          backgroundRemindersEnabled: true,
          accountRefreshEnabled: true,
          reminderPileCount: 0,
          reminderPileLimit: 5,
          dailyQuotaUsed: 0,
          dailyQuotaLimit: 480,
          refreshIntervalMinutes: 10,
          scheduledPowerOffEnabled: false,
        },
      })
    if (path === "/api/notifications")
      return route.fulfill({ json: { items: [], unreadCount: 0 } })
    if (path === "/api/shared-piles")
      return route.fulfill({ json: [] })
    // Unexpected requests are blocked rather than forwarded to a real account.
    return route.fulfill({ status: 404, json: { error: "Unmocked endpoint" } })
  })
}

test("mobile load failure offers retry instead of an empty account", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockApi(page, snapshot(), true)
  await page.goto("/dashboard/")
  const list = page.getByRole("complementary", { name: "常用充电桩列表" })
  await expect(list.getByText("暂时没有加载出来")).toBeVisible()
  await expect(list.getByText("添加第一个常用充电桩")).toHaveCount(0)
  await list.getByRole("button", { name: "重新加载" }).click()
  await expect(
    list.getByRole("button", { name: "查看 北门车棚" })
  ).toBeVisible()
})

for (const reason of ["partial", "disconnected"] as const) {
  test(`${reason} snapshots stay stale through port selection`, async ({
    page,
  }) => {
    const data = snapshot()
    if (reason === "partial") {
      data.refresh.partial = true
      data.refresh.failedDevices = 1
    } else data.piles[0].online = false
    await mockApi(page, data)
    await page.goto(`/dashboard/?pile=${data.piles[0].id}`)
    await page
      .getByRole("button", { name: "1 号充电口，上次空闲", exact: true })
      .click()
    await expect(
      page.getByText("上次读取为空闲，当前是否可用尚待确认。")
    ).toBeVisible()
    await expect(
      page.getByText("最近读取为空闲，使用前请在现场确认。")
    ).toHaveCount(0)
  })
}

test("unknown ports do not render a zero availability count", async ({
  page,
}) => {
  const data = snapshot()
  data.piles[0].ports = []
  await mockApi(page, data)
  await page.goto(`/dashboard/?pile=${data.piles[0].id}`)
  await expect(page.getByText("还没有端口状态")).toBeVisible()
  await expect(page.locator(".wb-availability-number")).toHaveText("尚未读取")
})

test("mobile selection, browser back and search preserve the list flow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 740 })
  await mockApi(page, snapshot())
  await page.goto("/dashboard/")
  await page.getByRole("button", { name: "查看 北门车棚" }).click()
  const detail = page.getByRole("article", { name: "充电桩详情" })
  await expect(detail).toBeVisible()
  await expect(
    detail.getByRole("button", { name: "1 号充电口，空闲", exact: true })
  ).toBeVisible()
  await detail
    .getByRole("button", { name: "1 号充电口，空闲", exact: true })
    .click()
  await expect(detail.getByRole("region", { name: "1 号口详情" })).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({
    path: "/tmp/charge-workbench-mobile.png",
    fullPage: true,
  })
  await page.goBack()
  await expect(detail.getByRole("region", { name: "1 号口详情" })).toHaveCount(0)
  await detail.getByRole("button", { name: "常用充电桩", exact: true }).click()
  await expect(detail).toHaveCount(0)
  await page
    .getByRole("textbox", { name: "搜索充电桩", exact: true })
    .fill("不存在")
  await expect(page.getByText("没有找到充电桩")).toBeVisible()
  await page.getByRole("button", { name: "清除筛选" }).click()
  await expect(
    page.getByRole("button", { name: "查看 北门车棚" })
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})

test("desktop detail remains usable in dark appearance", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 })
  await mockApi(page, snapshot())
  await page.goto("/dashboard/")
  await page
    .getByRole("button", { name: "1 号充电口，空闲", exact: true })
    .click()
  await page.getByRole("button", { name: "切换到深色模式" }).click()
  await expect(page.locator("html")).toHaveClass(/dark/)
  await expect(page.getByRole("region", { name: "1 号口详情" })).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  await page.screenshot({
    path: "/tmp/charge-workbench-desktop-dark.png",
    fullPage: true,
  })
})

test("account owns logout and groups settings on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockApi(page, snapshot())
  await page.goto("/dashboard/")
  await expect(
    page.getByRole("button", { name: "查看 北门车棚" })
  ).toBeVisible()
  await expect(page.getByRole("button", { name: "退出登录" })).toHaveCount(0)
  await page.goto("/account/?tab=appearance")
  await expect(page.getByRole("heading", { name: "界面外观" })).toBeVisible()
  await page.getByRole("button", { name: "退出登录", exact: true }).click()
  await expect(
    page.getByRole("dialog", { name: "退出当前账户？" })
  ).toBeVisible()
  await page.getByRole("button", { name: "取消", exact: true }).click()
  await expect(page.getByRole("dialog")).toHaveCount(0)
  expect(
    await page.evaluate(
      () => getComputedStyle(document.documentElement).overscrollBehaviorY
    )
  ).toBe("none")
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  await page.screenshot({
    path: "/tmp/charge-account-settings-mobile.png",
    fullPage: true,
  })
  await page.route("**/api/auth/logout", (route) =>
    route.fulfill({ status: 204 })
  )
  await page.getByRole("button", { name: "退出登录", exact: true }).click()
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "退出登录", exact: true })
    .click()
  await expect(page).toHaveURL(/\/login\/?$/)
})

for (const width of [390, 1440]) {
  test(`quick search layout and keyboard flow at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockApi(page, snapshot())
    await page.goto("/dashboard/")
    await expect(
      page.getByRole("button", { name: "查看 北门车棚" })
    ).toBeVisible()
    if (width === 390)
      await page.getByRole("button", { name: "切换到深色模式" }).click()
    await page.keyboard.press("Control+k")
    const dialog = page.getByRole("dialog", { name: "快速定位" })
    const input = dialog.getByRole("textbox", { name: "快速搜索" })
    await expect(input).toBeFocused()
    const title = await dialog
      .getByRole("heading", { name: "快速定位" })
      .boundingBox()
    const description = await dialog
      .getByText("搜索充电桩名称、桩号或页面。")
      .boundingBox()
    expect(description!.y).toBeGreaterThanOrEqual(title!.y + title!.height - 1)
    expect(description!.width).toBeGreaterThan(240)
    expect(
      await dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth
      )
    ).toBe(true)
    await input.fill("不存在的地方")
    await expect(
      dialog.getByText("没有找到结果，换个关键词试试。")
    ).toBeVisible()
    await input.press("ArrowDown")
    await input.fill("北门")
    await input.press("ArrowDown")
    await input.press("Enter")
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/pile=2601201412385560001/)
    await page.goBack()
    await expect(page).not.toHaveURL(/pile=/)
    await page.keyboard.press("Control+k")
    await expect(input).toBeFocused()
    await input.fill("")
    await page.screenshot({
      path: `/tmp/charge-search-${width}.png`,
      animations: "disabled",
    })
    await input.press("Escape")
    await expect(dialog).toHaveCount(0)
  })
}

for (const width of [390, 1440]) {
  test(`retired history links keep current ports and reminders at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    const data = snapshot()
    data.piles[0].ports[0] = {
      ...data.piles[0].ports[0],
      status: "in_use",
      usedText: "35 分钟",
      remainingText: "25 分钟",
    }
    await mockApi(page, data)
    const historyRequests: string[] = []
    page.on("request", (request) => {
      if (/\/api\/piles\/.*\/history/.test(request.url()))
        historyRequests.push(request.url())
    })
    await page.goto(
      `/dashboard/?pile=${data.piles[0].id}&port=1&detail=history`
    )
    const detail = page.getByRole("article", { name: "充电桩详情" })
    await expect(page.getByRole("region", { name: "1 号口详情" })).toBeVisible()
    await expect(detail.getByText("25 分钟")).toBeVisible()
    await expect(
      detail.getByText(/使用历史|查看这个口的历史|平均充电时长|占用率/)
    ).toHaveCount(0)
    await detail
      .getByRole("button", { name: "有空闲时提醒我", exact: true })
      .click()
    await expect(
      page.getByRole("dialog", { name: "有空闲时提醒我" })
    ).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await page
      .getByRole("button", { name: "1 号充电口，使用中", exact: true })
      .click()
    await expect(page.getByRole("region", { name: "1 号口详情" })).toHaveCount(
      0
    )
    if (width === 390) {
      await page
        .getByRole("button", { name: "常用充电桩", exact: true })
        .click()
      await expect(
        page.getByRole("button", { name: "查看 北门车棚" })
      ).toBeVisible()
    } else {
      await page.getByRole("button", { name: "切换到深色模式" }).click()
      await expect(page.locator("html")).toHaveClass(/dark/)
    }
    expect(historyRequests).toEqual([])
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    ).toBe(true)
    await page.screenshot({
      path: `/tmp/charge-without-history-${width}.png`,
      animations: "disabled",
    })
  })
}

function defaultQuota() {
  return {
    freeDailyLimit: 2,
    freeDailyUsed: 0,
    freeDailyRemaining: 2,
    bonusRemaining: 0,
    totalFreeRemaining: 2,
    contributedPileCount: 0,
    pileContributionBonusQuota: 2,
    sharedPileCacheTTLSeconds: 60,
    dailyResetAt: "2026-09-12T00:00:00+08:00",
    hasPersonalCookie: false,
    personalCookieStatus: "none" as const,
    joinedPublicPool: false,
    requirePoolShareForUnlimited: true,
    unlimitedActive: false,
    publicPoolAvailable: 1,
    freeUserWatchAllowed: false,
  }
}

async function mockCatalogAndCookieFlow(page: Page, data: DashboardSnapshot) {
  if (!data.quota) data.quota = defaultQuota()
  await mockApi(page, data)

  const catalog = [
    {
      deviceId: "2601201412385560001",
      number: "61034278",
      name: "北门车棚",
      address: "北门入口右侧",
      openNum: 10,
      portCount: 10,
      createdAt: data.updatedAt,
      updatedAt: data.updatedAt,
    },
    {
      deviceId: "2601201412385560002",
      number: "61034279",
      name: "南门东车棚",
      address: "南门主路东侧",
      openNum: 12,
      portCount: 12,
      createdAt: data.updatedAt,
      updatedAt: data.updatedAt,
    },
  ]
  const flow = {
    adds: 0,
    cookieUpdates: 0,
    poolJoins: 0,
    poolLeaves: 0,
    rejectAdd: false,
    nextSeq: 3,
  }
  let stamp = Date.parse(data.updatedAt) || Date.now()
  const bumpUpdatedAt = () => {
    stamp += 1000
    data.updatedAt = new Date(stamp).toISOString()
  }

  await page.route("**/api/shared-piles**", (route) => {
    const query = (new URL(route.request().url()).searchParams.get("q") || "").trim()
    const items = query
      ? catalog.filter((item) =>
          [item.name, item.number, item.address].some((field) => field.includes(query))
        )
      : catalog
    return route.fulfill({ json: items })
  })

  await page.route("**/api/piles", (route) => {
    if (route.request().method() !== "POST") return route.fulfill({ json: data })
    flow.adds++
    if (flow.rejectAdd) {
      return route.fulfill({
        status: 429,
        json: {
          code: "DAILY_PROBE_LIMIT_REACHED",
          error: "今日录桩校验次数已达上限，请明天再试。",
        },
      })
    }
    const body = route.request().postDataJSON() as Record<string, unknown>
    const fromCatalog = catalog.find(
      (item) => item.deviceId === body.id || item.number === body.number
    )
    const id = fromCatalog
      ? fromCatalog.deviceId
      : `260120141238556000${flow.nextSeq++}`
    const base = snapshot().piles[0]
    const pile = {
      ...base,
      id,
      number: String(body.number || fromCatalog?.number || ""),
      name: String(body.name || fromCatalog?.name || `充电桩 ${body.number || id.slice(-6)}`),
      address: String(body.address || fromCatalog?.address || ""),
      openNum: Number(body.openNum || fromCatalog?.openNum || 10),
    }
    if (!data.piles.some((item) => item.id === pile.id)) {
      data.piles.push(pile)
      data.statistics.pileCount = data.piles.length
    }
    const rewardGranted = fromCatalog ? 0 : data.quota!.pileContributionBonusQuota
    if (rewardGranted > 0 && data.quota) {
      data.quota = {
        ...data.quota,
        bonusRemaining: data.quota.bonusRemaining + rewardGranted,
        totalFreeRemaining: data.quota.totalFreeRemaining + rewardGranted,
        contributedPileCount: data.quota.contributedPileCount + 1,
      }
    }
    bumpUpdatedAt()
    return route.fulfill({
      status: 201,
      json: {
        ...pile,
        fromSharedCatalog: Boolean(fromCatalog),
        rewardGranted,
        quota: data.quota,
      },
    })
  })

  await page.route("**/api/session/cookie", (route) => {
    if (route.request().method() !== "POST") return route.fulfill({ status: 405 })
    flow.cookieUpdates++
    const body = route.request().postDataJSON() as {
      cookie?: string
      joinPublicPool?: boolean
    }
    const joined = Boolean(body.joinPublicPool)
    data.quota = {
      ...data.quota!,
      hasPersonalCookie: true,
      personalCookieStatus: "valid",
      joinedPublicPool: joined,
      unlimitedActive: joined || !data.quota!.requirePoolShareForUnlimited,
    }
    bumpUpdatedAt()
    return route.fulfill({ json: data })
  })

  await page.route("**/api/session/cookie/join-pool", (route) => {
    const joined = route.request().method() === "POST"
    if (joined) flow.poolJoins++
    else flow.poolLeaves++
    data.quota = {
      ...data.quota!,
      joinedPublicPool: joined,
      unlimitedActive: joined || !data.quota!.requirePoolShareForUnlimited,
    }
    bumpUpdatedAt()
    return route.fulfill({ json: data })
  })

  return flow
}

for (const width of [320, 390, 768, 1440]) {
  test(`shared catalog add and new pile contribution complete at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.emulateMedia({
      reducedMotion: "reduce",
      colorScheme: width === 390 || width === 1440 ? "dark" : "light",
    })
    await page.addInitScript(
      (theme) => localStorage.setItem("theme", theme),
      width === 390 || width === 1440 ? "dark" : "light"
    )
    const flow = await mockCatalogAndCookieFlow(page, snapshot())
    await page.goto("/dashboard/")
    await page
      .getByRole("button", { name: "添加充电桩", exact: true })
      .first()
      .click()
    const dialog = page.getByRole("dialog")
    await expect(
      dialog.getByRole("tab", { name: "从公共桩库添加" })
    ).toHaveAttribute("aria-selected", "true")
    await dialog
      .getByRole("textbox", { name: "搜索公共桩库", exact: true })
      .fill("南门")
    await expect(dialog.getByText("南门东车棚")).toBeVisible()
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)
    ).toBe(true)
    await page.screenshot({
      path: `/tmp/charge-catalog-${width}.png`,
      fullPage: true,
    })
    await dialog.getByRole("button", { name: "添加", exact: true }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/pile=2601201412385560002/)
    expect(flow.adds).toBe(1)

    await page.goBack()
    await expect(page).not.toHaveURL(/pile=/)

    await page
      .getByRole("button", { name: "添加充电桩", exact: true })
      .first()
      .click()
    await dialog.getByRole("tab", { name: /录入新桩/ }).click()
    const numberInput = dialog.getByLabel("桩号（推荐）")
    await expect(numberInput).toBeFocused()
    await numberInput.fill("61034280")
    await dialog.getByLabel("常用名称（可选）").fill("西门新车棚")
    await dialog.getByRole("button", { name: "校验并收录", exact: true }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/pile=2601201412385560003/)
    expect(flow.adds).toBe(2)
  })
}

test("add failure preserves contribute draft, offline disables submit, and Escape restores focus", async ({
  page,
  context,
}) => {
  const flow = await mockCatalogAndCookieFlow(page, snapshot())
  flow.rejectAdd = true
  await page.goto("/dashboard/")
  const trigger = page
    .getByRole("button", { name: "添加充电桩", exact: true })
    .first()
  await trigger.click()
  const dialog = page.getByRole("dialog")
  await dialog.getByRole("tab", { name: /录入新桩/ }).click()
  await dialog.getByLabel("桩号（推荐）").fill("61034280")
  await dialog.getByLabel("常用名称（可选）").fill("保留名称")
  const submit = dialog.getByRole("button", { name: "校验并收录", exact: true })
  await submit.click()
  await expect(dialog.getByRole("alert")).toContainText(
    "今日录桩校验次数已达上限"
  )
  await expect(dialog.getByLabel("常用名称（可选）")).toHaveValue("保留名称")
  expect(flow.adds).toBe(1)

  await context.setOffline(true)
  await expect(submit).toBeDisabled()
  await context.setOffline(false)
  await expect(submit).toBeEnabled()

  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()

  await trigger.click()
  await expect(dialog.getByLabel("常用名称（可选）")).toHaveValue("保留名称")
  flow.rejectAdd = false
  await dialog.getByRole("button", { name: "校验并收录", exact: true }).click()
  await expect(dialog).toHaveCount(0)
  expect(flow.adds).toBe(2)
})

test("account connection uploads personal cookie and toggles mutual-aid pool", async ({
  page,
}) => {
  const flow = await mockCatalogAndCookieFlow(page, snapshot())
  await page.goto("/account/?tab=connection&connect=1")
  const dialog = page.getByRole("dialog")
  await expect(
    dialog.getByRole("heading", { name: "上传个人 Cookie" })
  ).toBeVisible()
  await dialog
    .getByLabel("Cookie / cURL 内容")
    .fill("wxopenid=e2e-user-openid; info=test")
  await dialog.getByRole("button", { name: "保存并验证", exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByText("互助 Cookie · 不限次")).toBeVisible()
  expect(flow.cookieUpdates).toBe(1)

  await page.getByRole("button", { name: "退出互助池", exact: true }).click()
  await expect(page.getByText("未加入互助池 · 免费模式")).toBeVisible()
  expect(flow.poolLeaves).toBe(1)

  await page.getByRole("button", { name: "立即加入互助池", exact: true }).click()
  await expect(page.getByText("互助 Cookie · 不限次")).toBeVisible()
  expect(flow.poolJoins).toBe(1)
})

test("empty-list add entry opens the shared catalog dialog", async ({ page }) => {
  const data = snapshot()
  data.piles = []
  const flow = await mockCatalogAndCookieFlow(page, data)
  await page.goto("/dashboard/")
  const list = page.getByRole("complementary", { name: "常用充电桩列表" })
  await list.getByRole("button", { name: "添加充电桩", exact: true }).click()
  const dialog = page.getByRole("dialog")
  await expect(
    dialog.getByRole("tab", { name: "从公共桩库添加" })
  ).toHaveAttribute("aria-selected", "true")
  await expect(dialog.getByText("南门东车棚")).toBeVisible()
  expect(flow.adds).toBe(0)
})

for (const width of [320, 390, 768, 1440]) {
  test(`announcements acknowledge, revisit and retain important summaries at ${width}px`, async ({page}) => {
    await page.setViewportSize({width,height:900})
    await page.addInitScript((theme) => localStorage.setItem("theme", theme), width === 768 || width === 1440 ? "dark" : "light")
    await page.emulateMedia({reducedMotion:"reduce"})
    await mockApi(page,snapshot())
    const item = { id:"notice-1",title:"充电服务维护安排",body:"周五夜间维护，请提前安排。",level:"normal",status:"active",startAt:"2026-09-16T01:00:00Z",endAt:null,version:1,reminderVersion:1,acknowledged:false,createdAt:"2026-09-16T01:00:00Z",updatedAt:"2026-09-16T01:00:00Z" }
    await page.route("**/api/announcements**",route=>{
      if (route.request().method() !== "GET" && route.request().headers()["content-type"] !== "application/json") return route.fulfill({status:415,json:{error:"content type must be application/json"}})

      const path = new URL(route.request().url()).pathname
      if(path.endsWith("/summary"))return route.fulfill({json:{item:item.level==="important"||!item.acknowledged?{...item,body:undefined}:null,unreadCount:item.acknowledged?0:1,serverNow:new Date().toISOString(),nextBoundary:null}})
      if(path.endsWith("/acknowledge")){item.acknowledged=true;return route.fulfill({json:{ok:true}})}
      if(path.endsWith("notice-1"))return route.fulfill({json:item})
      return route.fulfill({json:{items:[item],total:1,page:1}})
    })
    await page.goto("/dashboard/?q=北门")
    const strip=page.getByRole("region",{name:"公告",exact:true})
    await expect(strip.getByText(item.title)).toBeVisible()
    await strip.getByRole("button",{name:"查看",exact:true}).click()
    const dialog=page.getByRole("dialog")
    await expect(dialog.getByText(item.body)).toBeVisible()
    if (width >= 768) expect((await dialog.getByRole("heading", {name:item.title}).boundingBox())!.y).toBeLessThan(180)
    await page.screenshot({path:`/tmp/charge-announcement-detail-${width}.png`,fullPage:true})
    await dialog.getByRole("button",{name:"我知道了"}).click()
    await expect(dialog.getByRole("button",{name:"已确认"})).toBeDisabled()
    await page.keyboard.press("Escape")
    await expect(strip.getByText(item.title)).toHaveCount(0)
    await expect(strip.getByRole("button", {name:"全部公告", exact:true})).toBeFocused()
    await page.reload()
    await expect(strip.getByText("暂无待确认公告")).toBeVisible()
    item.level="important"
    await page.reload()
    await expect(strip.getByText(item.title)).toBeVisible()
    await expect(strip.getByText(/已确认/)).toBeVisible()
    await strip.getByRole("button",{name:"查看",exact:true}).click()
    await expect(dialog.getByText(item.body)).toBeVisible()
    await page.goBack()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/q=/)
    await page.screenshot({path:`/tmp/charge-announcements-${width}.png`,fullPage:true})
    expect(await page.locator("body").evaluate(el=>el.scrollWidth<=window.innerWidth)).toBe(true)
  })
}

test("announcement outage does not block piles",async({page})=>{
  await mockApi(page,snapshot())
  await page.route("**/api/announcements/summary",route=>route.fulfill({status:503,json:{error:"公告暂时不可用"}}))
  await page.goto("/dashboard/")
  await expect(page.getByText("公告暂时无法加载")).toBeVisible()
  await expect(page.getByRole("button",{name:"刷新全部",exact:true})).toBeEnabled()
  await expect(page.getByRole("heading",{name:"北门车棚"})).toBeVisible()
})

test("administrator creates, previews, publishes and withdraws announcements",async({page})=>{
 await mockApi(page,snapshot())
 await page.route("**/api/auth/me",route=>route.fulfill({json:{id:"admin",username:"admin",role:"admin",enabled:true}}))
 let item:Record<string,unknown>|null=null
 await page.route("**/api/admin/announcements**",route=>{
      if (route.request().method() !== "GET" && route.request().headers()["content-type"] !== "application/json") return route.fulfill({status:415,json:{error:"content type must be application/json"}})

  const path=new URL(route.request().url()).pathname,method=route.request().method()
  if(method==="GET")return route.fulfill({json:path.endsWith("/a1")?item:{items:item?[item]:[],total:item?1:0,page:1}})
  if(path.endsWith("/publish")){item={...item,status:"active",version:2};return route.fulfill({json:item})}
  if(path.endsWith("/withdraw")){item={...item,status:"withdrawn",version:3};return route.fulfill({json:item})}
  item={...route.request().postDataJSON(),id:"a1",status:"draft",version:1,reminderVersion:1,acknowledged:false,createdAt:new Date().toISOString(),updatedAt:new Date().toISOString()};return route.fulfill({json:item})
 })
 await page.goto("/admin/?tab=announcements")
 await page.getByRole("button",{name:"新建公告",exact:true}).click()
 await page.getByLabel("标题",{exact:true}).fill("夜间维护")
 await page.getByLabel("正文",{exact:true}).fill("请提前安排充电。")
 await page.getByRole("button",{name:"预览",exact:true}).click()
 await expect(page.getByRole("dialog").getByText("请提前安排充电。")).toBeVisible()
 await page.keyboard.press("Escape")
 await page.getByRole("button",{name:"保存草稿",exact:true}).click()
 await page.getByRole("button",{name:"发布已保存草稿"}).click()
 await page.getByRole("dialog").getByRole("button",{name:"确认",exact:true}).click()
 await page.getByRole("button",{name:"撤下",exact:true}).click()
 await page.getByRole("dialog").getByRole("button",{name:"确认",exact:true}).click()
 await expect(page.getByRole("button",{name:"复制为新草稿"})).toBeVisible()
})


test("slow auxiliary requests do not hold the main workspace",async({page})=>{
 await mockApi(page,snapshot())
 let release!:()=>void
 const held = new Promise<void>(resolve=>{release=resolve})
 await page.route("**/api/notifications",async route=>{await held;await route.fulfill({json:{items:[],unreadCount:0}})})
 await page.route("**/api/watch-rules",async route=>{await held;await route.fulfill({json:[]})})
 await page.goto("/dashboard/?pile=2601201412385560001")
 await expect(page.getByRole("button",{name:"刷新全部",exact:true})).toBeEnabled()
 await expect(page.getByRole("button",{name:"有空闲时提醒我",exact:true})).toBeDisabled()
 release()
 await expect(page.getByRole("button",{name:"有空闲时提醒我",exact:true})).toBeEnabled()
})

for (const width of [320, 390, 768, 1440]) {
  test(`workspace actions and inline logout remain aligned at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockApi(page, snapshot())
    await page.goto("/dashboard/")
    const refresh = page.getByRole("button", { name: "刷新全部", exact: true })
    const add = page.getByRole("button", { name: "添加充电桩", exact: true })
    await expect(refresh).toBeEnabled()
    const refreshBox = await refresh.boundingBox()
    const addBox = await add.boundingBox()
    expect(refreshBox).not.toBeNull()
    expect(addBox).not.toBeNull()
    expect(Math.abs(refreshBox!.height - addBox!.height)).toBeLessThan(1)
    expect(Math.abs(refreshBox!.y - addBox!.y)).toBeLessThan(1)
    if (width < 761) {
      expect(refreshBox!.height).toBeGreaterThanOrEqual(44)
      expect(Math.abs(refreshBox!.width - addBox!.width)).toBeLessThan(1)
    }
    expect(await refresh.evaluate(e => getComputedStyle(e).fontSize)).toBe(
      await add.evaluate(e => getComputedStyle(e).fontSize)
    )
    await page.screenshot({ path: `/tmp/charge-actions-${width}.png`, fullPage: true })
    await page.goto("/account/?tab=appearance")
    const logout = page.getByRole("button", { name: "退出登录", exact: true })
    await expect(logout).toBeVisible()
    const profile = page.locator(".wb-profile-line")
    await expect(profile.getByRole("button", { name: "退出登录", exact: true })).toBeVisible()
    const profileBox = await profile.boundingBox()
    const logoutBox = await logout.boundingBox()
    expect(logoutBox!.y).toBeGreaterThanOrEqual(profileBox!.y)
    expect(logoutBox!.y + logoutBox!.height).toBeLessThanOrEqual(profileBox!.y + profileBox!.height)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `/tmp/charge-profile-${width}.png`, fullPage: true })
    await logout.click()
    await expect(page.getByRole("dialog", { name: "退出当前账户？" })).toBeVisible()
    await page.getByRole("button", { name: "取消", exact: true }).click()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await expect(logout).toBeFocused()
  })
}

for (const width of [320, 390, 768, 1440]) {
  test(`notification actions follow content flow at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockApi(page, snapshot())
    await page.addInitScript(() => {
      Object.defineProperty(Notification, "permission", { configurable: true, get: () => "default" })
    })
    await page.route("**/api/notification-channels/wxpusher", route => route.fulfill({ json: {
      configured: true, bound: false, enabled: false, eventTypes: [],
    } }))
    await page.goto("/account/?tab=notifications")
    const allow = page.getByRole("button", { name: "允许通知", exact: true })
    const qr = page.getByRole("button", { name: "获取二维码", exact: true })
    await expect(allow).toBeVisible()
    await expect(qr).toBeVisible()
    if (width === 390) await page.getByRole("button", { name: "切换到深色模式" }).click()
    const allowBox = (await allow.boundingBox())!
    const qrBox = (await qr.boundingBox())!
    const panel = (await page.locator(".wb-browser-notification").boundingBox())!
    const copy = (await page.locator(".wb-browser-notification-copy").boundingBox())!
    expect(allowBox.height).toBe(width <= 760 ? 44 : 35)
    expect(qrBox.height).toBe(allowBox.height)
    expect(await allow.evaluate(e => getComputedStyle(e).fontSize)).toBe(await qr.evaluate(e => getComputedStyle(e).fontSize))
    expect(allowBox.x).toBeGreaterThan(panel.x)
    expect(allowBox.x + allowBox.width).toBeLessThan(panel.x + panel.width)
    if (width <= 760) {
      expect(allowBox.y).toBeGreaterThanOrEqual(copy.y + copy.height)
    } else {
      expect(Math.abs(allowBox.y + allowBox.height / 2 - panel.y - panel.height / 2)).toBeLessThan(2)
      expect(Math.abs(allowBox.x + allowBox.width - qrBox.x - qrBox.width)).toBeLessThan(2)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await allow.focus()
    await expect(allow).toBeFocused()
    await page.screenshot({ path: `/tmp/charge-notification-actions-${width}.png`, fullPage: true })
  })
}

for (const width of [320, 1440]) {
  test(`administrator invite dialog keeps actions inside at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockApi(page, snapshot())
    await page.route("**/api/auth/me", route => route.fulfill({ json: { id: "admin", username: "admin", role: "admin", enabled: true } }))
    await page.route("**/api/admin/invites**", route => route.fulfill({ json: { items: [], total: 0, page: 1, pageSize: 20 } }))
    await page.goto("/admin/?tab=invites")
    await page.getByRole("button", { name: "创建邀请码", exact: true }).click()
    const dialog = page.getByRole("dialog", { name: "创建邀请码" })
    await expect(dialog).toBeVisible()
    await dialog.getByLabel("到期时间（本地时间）").fill("2027-01-01T12:00")
    expect(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true)
    const heights = await dialog.locator('button[data-ui-size="default"]').evaluateAll(es => es.map(e => parseFloat(getComputedStyle(e).height)))
    expect(heights).toEqual([width === 320 ? 44 : 35, width === 320 ? 44 : 35])
    await dialog.getByRole("button", { name: "取消", exact: true }).click()
    await expect(dialog).toHaveCount(0)
  })
}

for (const mode of ["login", "register"]) {
  for (const width of [320, 390, 1440]) {
    test(`${mode} password toggle stays inside the field while pressed at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 })
      let submissions = 0
      await page.route("**/api/auth/**", route => {
        if (route.request().method() === "POST") submissions++
        return route.fulfill({ json: { registrationOpen: true, loginCaptchaEnabled: false, registerCaptchaEnabled: false } })
      })
      await page.goto(`/${mode}/`)
      const input = page.getByLabel("密码", { exact: true })
      await input.fill("layout-test-password")
      const eye = page.getByRole("button", { name: "显示密码", exact: true })
      await expect(eye).toBeVisible()
      const before = (await eye.boundingBox())!
      await eye.hover()
      await page.mouse.down()
      await eye.evaluate(async element => {
        await new Promise(requestAnimationFrame)
        await Promise.all(element.getAnimations().map(animation => animation.finished.catch(() => {})))
      })
      await expect.poll(async () => {
        const button = (await eye.boundingBox())!
        const field = (await input.boundingBox())!
        return button.y >= field.y - 1 && button.y + button.height <= field.y + field.height + 1 && Math.abs(button.y - before.y) <= 1
      }).toBe(true)
      await page.mouse.up()
      await expect(input).toHaveAttribute("type", "text")
      await expect(input).toHaveValue("layout-test-password")
      const hide = page.getByRole("button", { name: "隐藏密码", exact: true })
      await hide.focus()
      await page.keyboard.press("Space")
      await expect(input).toHaveAttribute("type", "password")
      expect(submissions).toBe(0)
    })
  }
}

test("fluid layout adapts across critical viewport widths without overflow", async ({
  page,
}) => {
  const richSnapshot: DashboardSnapshot = {
    ...snapshot(),
    piles: [
      {
        ...snapshot().piles[0],
        openNum: 10,
        ports: Array.from({ length: 10 }, (_, idx) => ({
          id: idx + 1,
          status: idx % 3 === 0 ? "idle" : idx % 3 === 1 ? "in_use" : "offline",
          powerKw: idx % 3 === 1 ? 0.45 : 0,
          energyKwh: idx % 3 === 1 ? 1.2 : 0,
          sessionMin: idx % 3 === 1 ? 45 : 0,
          usedSeconds: idx % 3 === 1 ? 2700 : 0,
          remainingText: idx % 3 === 1 ? "1小时20分钟" : "",
          updatedAt: "2026-09-11T02:00:00Z",
        })),
      },
      {
        ...snapshot().piles[0],
        id: "2601201412385560002",
        number: "61034279",
        name: "南门地下车库西侧充电区",
        address: "南门地下二层 B 区柱子旁",
        openNum: 10,
        ports: Array.from({ length: 10 }, (_, idx) => ({
          id: idx + 1,
          status: idx % 2 === 0 ? "idle" : "in_use",
          powerKw: idx % 2 === 1 ? 0.32 : 0,
          energyKwh: idx % 2 === 1 ? 0.8 : 0,
          sessionMin: idx % 2 === 1 ? 30 : 0,
          usedSeconds: idx % 2 === 1 ? 1800 : 0,
          remainingText: idx % 2 === 1 ? "充满自停" : "",
          updatedAt: "2026-09-11T02:00:00Z",
        })),
      },
    ],
    quota: {
      freeDailyLimit: 20,
      freeDailyUsed: 3,
      freeDailyRemaining: 17,
      bonusRemaining: 10,
      totalFreeRemaining: 27,
      publicPoolAvailable: 3,
      contributedPileCount: 2,
      dailyResetAt: "2026-09-12T00:00:00+08:00",
      hasPersonalCookie: false,
      personalCookieStatus: "none",
      joinedPublicPool: false,
      requirePoolShareForUnlimited: true,
      unlimitedActive: false,
      freeUserWatchAllowed: false,
      pileContributionBonusQuota: 5,
      sharedPileCacheTTLSeconds: 60,
    },
  }

  await mockApi(page, richSnapshot)
  await page.route("**/api/watch-rules", (route) =>
    route.fulfill({
      json: [
        {
          id: "rule-1",
          deviceId: "2601201412385560001",
          mode: "temporary",
          enabled: true,
          expiresAt: "2026-09-11T05:00:00Z",
          nextCheckAt: "2026-09-11T02:10:00Z",
          createdAt: "2026-09-11T01:00:00Z",
        },
      ],
    })
  )
  const pageErrors: string[] = []
  page.on("pageerror", (error) => {
    pageErrors.push(error.message)
  })
  await page.route("**/api/notifications**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "notice-1",
            type: "pile_available",
            title: "北门车棚已有空闲充电口",
            message: "1 号口、4 号口当前空闲，可直接前往扫码充电。",
            deviceId: "2601201412385560001",
            portId: 1,
            createdAt: "2026-09-11T02:00:00Z",
            lastOccurredAt: "2026-09-11T02:00:00Z",
            occurrenceCount: 1,
          },
        ],
        unreadCount: 1,
      },
    })
  )

  const widths = [320, 390, 640, 765, 768, 850, 950, 1100, 1440]

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto("/dashboard/")
  await expect(
    page.getByRole("button", { name: "查看 北门车棚" })
  ).toBeVisible()

  for (const width of widths) {
    await page.setViewportSize({ width, height: 900 })
    const metrics = await page.evaluate(() => {
      const overflow =
        document.documentElement.scrollWidth > window.innerWidth + 1
      const searchBtn = document.querySelector(".wb-global-search")
      const searchSpan = document.querySelector(".wb-global-search > span")
      const searchSpanVisible =
        searchSpan instanceof HTMLElement &&
        getComputedStyle(searchSpan).display !== "none"
      const chips = Array.from(document.querySelectorAll(".wb-socket-chip"))
      const minChipWidth = chips.reduce(
        (min, el) => Math.min(min, el.getBoundingClientRect().width),
        Infinity
      )
      return {
        overflow,
        searchWidth: searchBtn?.getBoundingClientRect().width ?? 0,
        searchSpanVisible,
        minChipWidth,
      }
    })
    expect(metrics.overflow, `dashboard overflow at ${width}px`).toBe(false)
    expect(
      metrics.minChipWidth,
      `socket chip too narrow at ${width}px`
    ).toBeGreaterThanOrEqual(62)
    if (width >= 1024) {
      expect(
        metrics.searchSpanVisible,
        `search placeholder hidden at ${width}px`
      ).toBe(true)
    }
  }

  const routeChecks = [
    { path: "/dashboard/reminders/", readyText: "北门车棚" },
    { path: "/dashboard/notifications/", readyText: "1 号充电口空闲了" },
    { path: "/account/?tab=connection", readyText: "平台连接" },
  ]
  for (const { path, readyText } of routeChecks) {
    await page.goto(path)
    await expect(page.getByText(readyText).first()).toBeVisible()
    for (const width of [320, 390, 560, 768, 1100]) {
      await page.setViewportSize({ width, height: 900 })
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth + 1
      )
      expect(overflow, `${path} overflow at ${width}px`).toBe(false)
    }
  }
  expect(pageErrors, `unexpected page errors: ${pageErrors.join("; ")}`).toEqual(
    []
  )
})

test("admin user filters and header stay inside bounds across critical widths", async ({
  page,
}) => {
  await mockApi(page, snapshot())
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({
      json: {
        id: "admin",
        username: "admin",
        role: "admin",
        enabled: true,
        refreshEnabled: true,
      },
    })
  )
  await page.route("**/api/admin/users**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            user: {
              id: "u1",
              username: "student_a",
              role: "user",
              enabled: true,
              refreshEnabled: true,
              deviceLimit: 10,
              createdAt: "2026-09-01T00:00:00Z",
            },
            dashboard: {
              pileCount: 3,
              portCount: 30,
              idlePortCount: 12,
              inUsePortCount: 15,
              offlinePorts: 3,
            },
            lastRefresh: {
              lastRemoteAt: "2026-09-11T02:00:00Z",
              failedDevices: 0,
            },
          },
        ],
        total: 1,
        page: 1,
        pageSize: 15,
        totalPages: 1,
      },
    })
  )

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto("/admin/?tab=users")
  await expect(page.getByRole("heading", { name: "筛选用户" })).toBeVisible()

  for (const width of [320, 390, 640, 765, 768, 850, 950, 1024, 1100, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    const result = await page.evaluate(() => {
      const pageOverflow =
        document.documentElement.scrollWidth > window.innerWidth + 1
      const filterSection = document.querySelector(
        'section[aria-label="用户筛选"]'
      )
      const filterOverflow = filterSection
        ? filterSection.scrollWidth > filterSection.clientWidth + 1
        : false
      const header = document.querySelector(".wb-header-inner")
      const headerOverflow = header
        ? header.scrollWidth > header.clientWidth + 1
        : false
      return { pageOverflow, filterOverflow, headerOverflow }
    })
    expect(result.pageOverflow, `admin page overflow at ${width}px`).toBe(false)
    expect(result.filterOverflow, `admin filter overflow at ${width}px`).toBe(
      false
    )
    expect(result.headerOverflow, `admin header overflow at ${width}px`).toBe(
      false
    )
  }
})

test("mobile topbar controls stay aligned and drawer auto-closes on desktop resize", async ({
  page,
}) => {
  await mockApi(page, snapshot())
  await page.setViewportSize({ width: 560, height: 844 })
  await page.goto("/dashboard/")
  await expect(
    page.getByRole("button", { name: "查看 北门车棚" })
  ).toBeVisible()

  const topbarMetrics = await page.evaluate(() => {
    const dot = document.querySelector(".wb-conn-chip .wb-pulse-dot")
    const dotRect = dot?.getBoundingClientRect()
    const searchRect = document
      .querySelector(".wb-global-search")
      ?.getBoundingClientRect()
    const rightRect = document
      .querySelector(".wb-header-right")
      ?.getBoundingClientRect()
    const heights = [
      document.querySelector(".wb-mobile-menu-trigger button"),
      document.querySelector(".wb-global-search"),
      document.querySelector(".wb-header-icon-link"),
      document.querySelector('.wb-header-right button[data-ui-button]'),
      document.querySelector(".wb-avatar-link"),
    ]
      .filter((el): el is Element => Boolean(el))
      .map((el) => el.getBoundingClientRect().height)
    return {
      dotVisible: Boolean(dotRect && dotRect.width > 0 && dotRect.height > 0),
      searchToRightGap:
        searchRect && rightRect ? rightRect.left - searchRect.right : 999,
      minHeight: Math.min(...heights),
      maxHeight: Math.max(...heights),
    }
  })

  expect(topbarMetrics.dotVisible).toBe(true)
  expect(topbarMetrics.searchToRightGap).toBeLessThanOrEqual(12)
  expect(topbarMetrics.maxHeight - topbarMetrics.minHeight).toBeLessThanOrEqual(
    2.5
  )
  expect(topbarMetrics.maxHeight).toBeLessThanOrEqual(36)

  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole("button", { name: "打开菜单" }).click()
  const drawer = page.locator('[data-slot="sheet-content"]')
  await expect(drawer).toBeVisible()

  const drawerMetrics = await page.evaluate(() => {
    const sheet = document.querySelector('[data-slot="sheet-content"]')
    const closeBtn = sheet?.querySelector('[data-slot="sheet-close"]')
    const account = sheet?.querySelector(".wb-menu-account")
    const sheetRect = sheet?.getBoundingClientRect()
    const closeRect = closeBtn?.getBoundingClientRect()
    const accountRect = account?.getBoundingClientRect()
    return {
      sheetWidth: sheetRect?.width ?? 0,
      closeHeight: closeRect?.height ?? 0,
      accountBottomGap:
        sheetRect && accountRect ? sheetRect.bottom - accountRect.bottom : 999,
    }
  })

  expect(drawerMetrics.sheetWidth).toBeLessThanOrEqual(302)
  expect(drawerMetrics.closeHeight).toBeLessThanOrEqual(34)
  expect(drawerMetrics.accountBottomGap).toBeLessThanOrEqual(24)

  await page.setViewportSize({ width: 1024, height: 844 })
  await expect(drawer).toHaveCount(0)
})
