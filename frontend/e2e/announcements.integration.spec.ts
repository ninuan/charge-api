import { expect, test, type Page } from "@playwright/test"

async function login(page: Page, username: string, password: string) {
  await page.goto("/login/")
  await page.getByLabel("用户名", { exact: true }).fill(username)
  await page.getByLabel("密码", { exact: true }).fill(password)
  await page.getByRole("button", { name: "登录", exact: true }).click()
  await expect(page).not.toHaveURL(/\/login/)
}

test("real server accepts announcement create, publish, edit, acknowledge and withdraw", async ({
  page,
  browser,
}) => {
  await login(page, "admin", "local-announcement-test-password")
  await page.goto("/admin/?tab=announcements")
  await page.getByRole("button", { name: "新建公告", exact: true }).click()
  await page.getByLabel("标题", { exact: true }).fill("真实接口公告")
  await page
    .getByRole("textbox", { name: "正文", exact: true })
    .fill("验证真实后端与数据库。")
  const created = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/admin/announcements") &&
      r.request().method() === "POST"
  )
  await page.getByRole("button", { name: "保存草稿", exact: true }).click()
  const response = await created
  expect(response.status(), await response.text()).toBe(200)
  expect(response.request().headers()["content-type"]).toBe("application/json")
  const item = await response.json()
  await page.getByRole("button", { name: "发布已保存草稿" }).click()
  const published = page.waitForResponse((r) =>
    r.url().endsWith(`/${item.id}/publish`)
  )
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确认", exact: true })
    .click()
  expect((await published).status()).toBe(200)
  await expect(
    page.getByRole("button", { name: "撤下", exact: true })
  ).toBeVisible()
  const user = await page.request.post("/api/admin/users", {
    data: {
      username: "announcement-reader",
      password: "reader-password-123",
      role: "user",
    },
  })
  expect(user.ok(), await user.text()).toBe(true)
  const context = await browser.newContext({
    baseURL: "http://127.0.0.1:18081",
  })
  try {
    const reader = await context.newPage()
    await login(reader, "announcement-reader", "reader-password-123")
    const guide = reader.getByRole("dialog", {
      name: "Charge Console 使用说明",
    })
    await expect(guide).toBeVisible()
    await guide.locator(".overflow-y-auto").evaluate((element) => {
      element.scrollTop = element.scrollHeight
    })
    await guide.getByRole("button", { name: "我已看完并关闭" }).click()
    await expect(guide).toBeHidden()
    const strip = reader.getByRole("region", { name: "公告", exact: true })
    await strip.getByRole("button", { name: "查看", exact: true }).click()
    const ack = reader.waitForResponse((r) =>
      r.url().endsWith(`/${item.id}/acknowledge`)
    )
    await reader
      .getByRole("dialog")
      .getByRole("button", { name: "我知道了", exact: true })
      .click()
    expect((await ack).status()).toBe(200)
    await reader.keyboard.press("Escape")
    await expect(strip.getByText("真实接口公告")).toHaveCount(0)
    await reader.reload()
    await expect(strip.getByText("暂无待确认公告")).toBeVisible()
    await page.bringToFront()
    await page
      .getByRole("textbox", { name: "正文", exact: true })
      .fill("更新正文，重新提醒。")
    await page.getByRole("button", { name: "更新并重新提醒" }).click()
    const edited = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/api/admin/announcements/${item.id}`) &&
        r.request().method() === "PATCH"
    )
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "确认", exact: true })
      .click()
    expect((await edited).status()).toBe(200)
    await reader.reload()
    await expect(strip.getByText("真实接口公告")).toBeVisible()
    await page.getByRole("button", { name: "撤下", exact: true }).click()
    const withdrawn = page.waitForResponse((r) =>
      r.url().endsWith(`/${item.id}/withdraw`)
    )
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "确认", exact: true })
      .click()
    expect((await withdrawn).status()).toBe(200)
    await reader.reload()
    await expect(strip.getByText("真实接口公告")).toHaveCount(0)
    await page.bringToFront()
    const copiedResponse = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/admin/announcements") &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "复制为新草稿" }).click()
    const copied = await copiedResponse
    expect(copied.status()).toBe(200)
    const draft = await copied.json()
    await expect(
      page.getByRole("textbox", { name: "正文", exact: true })
    ).toHaveValue("更新正文，重新提醒。")
    await page.getByRole("button", { name: "删除草稿" }).click()
    const deleted = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/api/admin/announcements/${draft.id}`) &&
        r.request().method() === "DELETE"
    )
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "确认", exact: true })
      .click()
    expect((await deleted).status()).toBe(200)
    await expect(
      page.getByRole("button", { name: "新建公告", exact: true })
    ).toBeVisible()
  } finally {
    await context.close()
  }
})
