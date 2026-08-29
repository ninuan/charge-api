import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { NotificationCenter } from "@/components/notification-center"

const mocks = vi.hoisted(() => ({
  items: [] as Array<Record<string, unknown>>,
  permission: "default" as NotificationPermission | "unsupported",
  markRead: vi.fn(),
  load: vi.fn(),
  loadMore: vi.fn(),
  markAllRead: vi.fn(),
  clearResolved: vi.fn(),
  requestPermission: vi.fn(),
  setBrowserEnabled: vi.fn(),
}))

const notice = {
  id: "notice-1",
  userId: "user-1",
  type: "pile_available" as const,
  severity: "info" as const,
  title: "松园充电桩有空闲口",
  message: "整桩出现至少一个空闲充电口",
  deviceId: "pile-1",
  portId: 3,
  createdAt: "2026-08-11T09:00:00Z",
}

vi.mock("@/lib/notification-context", () => ({
  useNotifications: () => ({
    items: mocks.items,
    unreadCount: 1,
    status: "all",
    loading: false,
    loadingMore: false,
    loaded: true,
    error: null,
    browserPermission: mocks.permission,
    load: mocks.load,
    loadMore: mocks.loadMore,
    markRead: mocks.markRead,
    markAllRead: mocks.markAllRead,
    clearResolved: mocks.clearResolved,
    requestBrowserPermission: mocks.requestPermission,
    setBrowserEnabled: mocks.setBrowserEnabled,
  }),
}))

vi.mock("@/lib/watch-context", () => ({
  useWatch: () => ({
    preference: {
      browserEnabled: false,
      quietHoursEnabled: true,
      quietStartMinute: 1320,
      quietEndMinute: 480,
      timezone: "Asia/Shanghai",
    },
  }),
}))

describe("NotificationCenter", () => {
  beforeEach(() => {
    mocks.items = [notice]
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    mocks.permission = "default"
  })

  it("shows unread state, permission guidance, and opens a notification target", async () => {
    mocks.markRead.mockResolvedValue(undefined)
    const onNavigate = vi.fn()
    const user = userEvent.setup()
    render(<NotificationCenter piles={[]} onNavigate={onNavigate} />)

    await user.click(screen.getByRole("button", { name: "通知，1 条未读" }))
    expect(await screen.findByText("通知中心")).toBeVisible()
    expect(screen.getByRole("button", { name: "允许通知" })).toBeVisible()
    expect(screen.getByText("网页开着时提醒我")).toBeVisible()
    expect(screen.getByText("来源：空闲提醒")).toBeVisible()
    expect(screen.getByText("建议：现在有空闲口，可以前往充电。")).toBeVisible()
    await user.click(screen.getByText("3 号充电口空闲了"))

    await waitFor(() => expect(mocks.markRead).toHaveBeenCalledWith("notice-1"))
    expect(onNavigate).toHaveBeenCalledWith(notice)
  })

  it("keeps informational notices out of the action lifecycle badges", async () => {
    const user = userEvent.setup()
    render(<NotificationCenter piles={[]} onNavigate={vi.fn()} />)
    await user.click(screen.getByRole("button", { name: "通知，1 条未读" }))

    const item = screen.getByRole("article")
    expect(within(item).queryByText("需处理")).toBeNull()
    expect(within(item).queryByText("已恢复")).toBeNull()
  })

  it("shows action lifecycle badges only for actionable problems", async () => {
    mocks.items = [
      {
        ...notice,
        id: "notice-actionable",
        type: "credential_expired",
        title: "需要重新登录",
      },
    ]
    const user = userEvent.setup()
    render(<NotificationCenter piles={[]} onNavigate={vi.fn()} />)
    await user.click(screen.getByRole("button", { name: "通知，1 条未读" }))

    expect(
      within(screen.getByRole("article")).getByText("需处理")
    ).toBeVisible()
  })

  it("explains denied and unsupported browser notification states", async () => {
    const user = userEvent.setup()
    mocks.permission = "denied"
    const { unmount } = render(
      <NotificationCenter piles={[]} onNavigate={vi.fn()} />
    )
    await user.click(screen.getByRole("button", { name: "通知，1 条未读" }))
    expect(screen.getByText("浏览器通知权限已被拒绝")).toBeVisible()
    unmount()

    mocks.permission = "unsupported"
    render(<NotificationCenter piles={[]} onNavigate={vi.fn()} />)
    await user.click(screen.getByRole("button", { name: "通知，1 条未读" }))
    expect(screen.getByText("当前浏览器不支持网页通知")).toBeVisible()
  })
})
