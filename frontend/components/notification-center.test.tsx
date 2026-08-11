import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { NotificationCenter } from "@/components/notification-center"

const mocks = vi.hoisted(() => ({
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
    items: [notice],
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
    await user.click(screen.getByText("松园充电桩有空闲口"))

    await waitFor(() => expect(mocks.markRead).toHaveBeenCalledWith("notice-1"))
    expect(onNavigate).toHaveBeenCalledWith(notice)
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
