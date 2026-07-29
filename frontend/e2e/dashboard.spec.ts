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
  await page.getByRole("button", { name: "保存设置" }).click()
  await expect(page.getByText("系统设置已保存")).toBeVisible()
  await page.getByRole("tab", { name: "运维审计" }).click()
  await expect(page.getByText("数据与备份")).toBeVisible()
  await expect(page.getByText("管理操作日志")).toBeVisible()
  await expect(page.getByText(/修改账户状态/).first()).toBeVisible()
  await expect(page.getByText(/重置密码/).first()).toBeVisible()
  await expect(page.getByText(/修改系统设置/).first()).toBeVisible()
  await page.screenshot({
    path: "/tmp/charge-1.4.13-admin-operations.png",
    fullPage: false,
  })

  await page.setViewportSize({ width: 375, height: 812 })
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
