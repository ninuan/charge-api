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
  await expect(
    page.getByRole("button", { name: "1 号充电口，空闲", exact: true })
  ).toBeVisible()
  await page
    .getByRole("button", { name: "1 号充电口，空闲", exact: true })
    .click()
  await expect(page.getByRole("region", { name: "1 号口详情" })).toBeVisible()
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
  await expect(page.getByRole("region", { name: "1 号口详情" })).toHaveCount(0)
  await page.getByRole("button", { name: "常用充电桩", exact: true }).click()
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
