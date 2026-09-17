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

async function mockBindingFlow(
  page: Page,
  data: DashboardSnapshot,
  initiallyBound: boolean,
  scanEnabled = true
) {
  await mockApi(page, data)
  const flow = {
    status: "pending",
    bound: initiallyBound,
    adds: 0,
    confirms: 0,
    creates: 0,
    polls: 0,
    rejectAdd: false,
    failBinding: false,
  }
  await page.route("**/api/session/yyb-binding", (route) =>
    route.fulfill({
      status: flow.failBinding ? 503 : 200,
      json: { bound: flow.bound, scanEnabled },
    })
  )
  await page.route("**/api/session/yyb-qr", (route) => {
    flow.creates++
    return route.fulfill({
      json: {
        sessionId: "browser-qr",
        imageBase64:
          "data:image/svg+xml;base64," +
          Buffer.from(
            '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200"><rect width="200" height="200" fill="white"/><path d="M20 20h50v50H20zm110 0h50v50h-50zM20 130h50v50H20z" fill="black"/></svg>'
          ).toString("base64"),
      },
    })
  })
  await page.route("**/api/session/yyb-qr/*/poll", (route) => {
    flow.polls++
    return route.fulfill({ json: { sessionId: "browser-qr", status: flow.status } })
  })
  await page.route("**/api/session/yyb-qr/*/confirm", (route) => {
    flow.confirms++
    flow.bound = true
    return route.fulfill({
      json: {
        bound: true,
        sessionId: "browser-qr",
        cookieSynced: false,
        syncState: "not_needed",
      },
    })
  })
  await page.route("**/api/piles", (route) => {
    if (route.request().method() !== "POST")
      return route.fulfill({ json: data })
    flow.adds++
    if (flow.rejectAdd)
      return route.fulfill({
        status: 409,
        json: { code: "YYB_RESCAN_REQUIRED" },
      })
    const pile = {
      ...data.piles[0],
      ...route.request().postDataJSON(),
      id: "2601201412385560002",
    }
    data.piles.push(pile)
    return route.fulfill({ status: 201, json: pile })
  })
  return flow
}

test("automatic checks preserve the QR until delayed phone confirmation", async ({ page }) => {
  const flow = await mockBindingFlow(page, snapshot(), false)
  await page.goto("/dashboard/")
  await page.getByRole("button", { name: "添加充电桩", exact: true }).first().click()
  const dialog = page.getByRole("dialog")
  const confirm = dialog.getByRole("button", { name: "确认绑定", exact: true })
  // Let multiple real automatic timer cycles finish before authorizing on the phone.
  await expect.poll(() => flow.polls, { timeout: 15000 }).toBeGreaterThanOrEqual(2)
  await expect(confirm).toBeDisabled()
  flow.status = "scanned"
  await expect(dialog.getByText("已扫码，请在微信中确认授权")).toBeVisible({ timeout: 10000 })
  await expect(confirm).toBeDisabled()
  flow.status = "authorized"
  await dialog.getByRole("button", { name: "检查扫码状态" }).click()
  await expect(confirm).toBeEnabled()
  await expect(dialog.getByText("扫码会话不匹配，请重新检查。")).toHaveCount(0)
  await confirm.click()
  await expect(dialog.getByText("微信已绑定", { exact: true })).toBeVisible()
  expect(flow.creates).toBe(1)
  expect(flow.confirms).toBe(1)
})

for (const width of [320, 390, 768, 1440]) {
  test(`binding onboarding completes at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await page.emulateMedia({
      reducedMotion: "reduce",
      colorScheme: width === 390 || width === 1440 ? "dark" : "light",
    })
    await page.addInitScript(
      (theme) => localStorage.setItem("theme", theme),
      width === 390 || width === 1440 ? "dark" : "light"
    )
    const flow = await mockBindingFlow(page, snapshot(), false)
    await page.goto("/dashboard/")
    await page
      .getByRole("button", { name: "添加充电桩", exact: true })
      .first()
      .click()
    const dialog = page.getByRole("dialog")
    await expect(
      dialog.getByRole("heading", { name: "添加前，先绑定微信" })
    ).toBeVisible()
    await expect(
      dialog.getByRole("img", { name: "微信扫码登录二维码" })
    ).toBeVisible()
    const confirm = dialog.getByRole("button", {
      name: "确认绑定",
      exact: true,
    })
    await expect(confirm).toBeDisabled()
    flow.status = "scanned"
    await dialog.getByRole("button", { name: "检查扫码状态" }).click()
    await expect(dialog.getByText("已扫码，请在微信中确认授权")).toBeVisible()
    await expect(confirm).toBeDisabled()
    flow.status = "authorized"
    await dialog.getByRole("button", { name: "检查扫码状态" }).click()
    await expect(confirm).toBeEnabled()
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)
    ).toBe(true)
    await page.screenshot({
      path: `/tmp/charge-binding-${width}.png`,
      fullPage: true,
    })
    await confirm.focus()
    await page.keyboard.press("Enter")
    await expect(dialog.getByText("微信已绑定", { exact: true })).toBeVisible()
    expect(flow.adds).toBe(0)
    await dialog.getByRole("button", { name: "继续添加充电桩" }).click()
    await expect(dialog.getByLabel("桩号", { exact: true })).toBeFocused()
    await dialog.getByLabel("桩号", { exact: true }).fill("61034279")
    await dialog.getByLabel("显示名称").fill("新绑定车棚")
    await dialog.getByRole("button", { name: "确认添加", exact: true }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page).toHaveURL(/pile=2601201412385560002/)
    expect(flow.adds).toBe(1)
    expect(flow.confirms).toBe(1)
    expect(flow.creates).toBe(1)
  })
}

test("rescan after add failure preserves the draft and never replays the add", async ({
  page,
}) => {
  const flow = await mockBindingFlow(page, snapshot(), true)
  flow.rejectAdd = true
  await page.goto("/dashboard/")
  await page
    .getByRole("button", { name: "添加充电桩", exact: true })
    .first()
    .click()
  const dialog = page.getByRole("dialog")
  await dialog.getByLabel("桩号", { exact: true }).fill("61034279")
  await dialog.getByLabel("显示名称").fill("保留名称")
  await dialog.getByRole("button", { name: "确认添加", exact: true }).click()
  await dialog.getByRole("button", { name: "去绑定 / 重新扫码" }).click()
  await expect(dialog.getByRole("img")).toBeVisible()
  await dialog.getByRole("button", { name: "返回填写" }).click()
  await expect(dialog.getByLabel("显示名称")).toHaveValue("保留名称")
  await expect(
    dialog.getByRole("button", { name: "确认添加", exact: true })
  ).toBeDisabled()
  await dialog.getByRole("button", { name: "去绑定 / 重新扫码" }).click()
  await expect(dialog.getByRole("img")).toBeVisible()
  flow.status = "authorized"
  await dialog.getByRole("button", { name: "检查扫码状态" }).click()
  await dialog.getByRole("button", { name: "确认绑定", exact: true }).click()
  await expect(
    dialog.getByRole("button", { name: "继续添加充电桩" })
  ).toBeVisible()
  await expect(dialog.getByRole("button", { name: "返回填写" })).toHaveCount(0)
  await dialog.getByRole("button", { name: "继续添加充电桩" }).click()
  expect(flow.adds).toBe(1)
  await expect(dialog.getByLabel("显示名称")).toHaveValue("保留名称")
  flow.rejectAdd = false
  await dialog.getByRole("button", { name: "确认添加", exact: true }).click()
  await expect(dialog).toHaveCount(0)
  expect(flow.adds).toBe(2)
})

test("binding check failure, offline and manual Cookie have actionable recovery", async ({
  page,
  context,
}) => {
  const flow = await mockBindingFlow(page, snapshot(), false, false)
  flow.failBinding = true
  await page.goto("/dashboard/")
  const trigger = page
    .getByRole("button", { name: "添加充电桩", exact: true })
    .first()
  await trigger.click()
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByRole("button", { name: "重新检查" })).toBeVisible()
  expect(flow.creates).toBe(0)
  flow.failBinding = false
  await dialog.getByRole("button", { name: "重新检查" }).click()
  await expect(dialog.getByLabel("桩号", { exact: true })).toBeVisible()
  await dialog.getByRole("button", { name: "手动设置 Cookie" }).click()
  await expect(dialog.getByLabel("手动更新 Cookie")).toBeVisible()
  await context.setOffline(true)
  await expect(
    dialog.getByRole("button", { name: "确认添加", exact: true })
  ).toBeDisabled()
  await context.setOffline(false)
  await page.keyboard.press("Escape")
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()
  expect(flow.creates).toBe(0)
})

test("account rebind polls a new QR even while the old binding exists", async ({
  page,
}) => {
  const flow = await mockBindingFlow(page, snapshot(), true)
  await page.goto("/account/?tab=connection&connect=1")
  const dialog = page.getByRole("dialog")
  await expect(
    dialog.getByRole("heading", { name: "绑定平台微信" })
  ).toBeVisible()
  await dialog.getByRole("button", { name: "重新扫码绑定" }).click()
  await expect(dialog.getByRole("img")).toBeVisible()
  flow.status = "authorized"
  await expect(
    dialog.getByRole("button", { name: "确认绑定", exact: true })
  ).toBeEnabled({ timeout: 10000 })
  await dialog.getByRole("button", { name: "确认绑定", exact: true }).click()
  await expect(dialog.getByText("微信已绑定", { exact: true })).toBeVisible()
  await dialog.getByRole("button", { name: "完成", exact: true }).click()
  await expect(dialog).toHaveCount(0)
  expect(flow.confirms).toBe(1)
  expect(flow.adds).toBe(0)
})

test("empty-list add entry opens the same binding flow", async ({ page }) => {
  const data = snapshot()
  data.piles = []
  const flow = await mockBindingFlow(page, data, false)
  await page.goto("/dashboard/")
  const list = page.getByRole("complementary", { name: "常用充电桩列表" })
  await list.getByRole("button", { name: "添加充电桩", exact: true }).click()
  await expect(page.getByRole("dialog").getByRole("img")).toBeVisible()
  expect(flow.creates).toBe(1)
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
  await expect(page.getByRole("button",{name:"刷新状态",exact:true})).toBeEnabled()
  await expect(page.getByRole("heading",{name:"北门车棚"})).toBeVisible()
})

test("administrator creates, previews, publishes and withdraws announcements",async({page})=>{
 await mockApi(page,snapshot())
 await page.route("**/api/auth/me",route=>route.fulfill({json:{id:"admin",username:"admin",role:"admin",enabled:true}}))
 let item:Record<string,unknown>|null=null
 await page.route("**/api/admin/announcements**",route=>{
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
 await expect(page.getByRole("button",{name:"刷新状态",exact:true})).toBeEnabled()
 await expect(page.getByRole("button",{name:"有空闲时提醒我",exact:true})).toBeDisabled()
 release()
 await expect(page.getByRole("button",{name:"有空闲时提醒我",exact:true})).toBeEnabled()
})
