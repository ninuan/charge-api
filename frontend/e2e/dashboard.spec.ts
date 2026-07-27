import { expect, test } from "@playwright/test"

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
  status: index === 2 ? "idle" : index === 4 ? "offline" : "in_use",
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

test("administrator signs in and filters the user list", async ({ page }) => {
  await page.goto("/login")
  await page.getByLabel("用户名").fill("admin")
  await page.locator("#password").fill("localadmin123")
  await page.getByRole("button", { name: "登录", exact: true }).click()

  await expect(page).toHaveURL(/\/admin\/?$/)
  await expect(page.getByRole("heading", { name: "运营总览" })).toBeVisible()
  await page.getByRole("tab", { name: "用户管理" }).click()
  await expect(page).toHaveURL(/\/admin\/?\?tab=users$/)
  await expect(page.getByRole("heading", { name: "筛选用户" })).toBeVisible()
  await page.getByPlaceholder("用户名").fill("admin")
  await page.getByRole("button", { name: "应用筛选" }).click()
  await expect(
    page.getByLabel("用户目录").getByText("admin", { exact: true })
  ).toBeVisible()

  await page.screenshot({
    path: "/tmp/charge-1.4.12-admin-users.png",
    fullPage: false,
  })

  await page.setViewportSize({ width: 375, height: 812 })
  await page.getByRole("tab", { name: "运营总览" }).click()
  await expect(page.getByRole("heading", { name: "运营总览" })).toBeVisible()
  const issueMetric = await page.getByLabel("待处理问题 指标").boundingBox()
  const successMetric = await page.getByLabel("远端成功率 指标").boundingBox()
  const activeMetric = await page.getByLabel("活跃用户 指标").boundingBox()
  expect(successMetric?.y).toBeGreaterThan(issueMetric?.y ?? 0)
  expect(successMetric?.y).toBe(activeMetric?.y)

  await page.screenshot({
    path: "/tmp/charge-1.4.12-admin-mobile.png",
    fullPage: false,
  })
})

test("dashboard restores URL filters and keeps the mobile controls clear", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 })
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

  await page.goto("/dashboard?q=03&status=idle")

  const search = page.getByPlaceholder("名称、桩号、地址或端口号")
  await expect(search).toHaveValue("03")
  await expect(search).toHaveCSS("padding-left", "40px")
  await expect(page.getByText("03", { exact: true })).toBeVisible()
  await expect(page.getByText("等待使用")).toBeVisible()
  await expect(page.getByText("设备暂不可访问")).toHaveCount(0)

  const firstMetric = await page.getByLabel("充电桩 指标").boundingBox()
  const secondMetric = await page.getByLabel("全部端口 指标").boundingBox()
  const thirdMetric = await page.getByLabel("正在使用 指标").boundingBox()
  expect(firstMetric?.y).toBe(secondMetric?.y)
  expect(thirdMetric?.y).toBeGreaterThan(firstMetric?.y ?? 0)

  await search.fill("松园")
  await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe("松园")
  await expect(page.getByText("03", { exact: true })).toBeVisible()

  await page.screenshot({
    path: "/tmp/charge-1.4.12-dashboard-mobile.png",
    fullPage: false,
  })
})
