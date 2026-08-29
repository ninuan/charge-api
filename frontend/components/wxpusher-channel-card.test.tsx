import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { WxPusherChannelCard } from "@/components/wxpusher-channel-card"

const api = vi.hoisted(() => ({
  getChannel: vi.fn(),
  createSession: vi.fn(),
  pollSession: vi.fn(),
  deleteChannel: vi.fn(),
  updateChannel: vi.fn(),
  testChannel: vi.fn(),
  recheckTest: vi.fn(),
}))

vi.mock("@/lib/wxpusher-api", () => ({
  getWxPusherChannel: api.getChannel,
  createWxPusherBindSession: api.createSession,
  pollWxPusherBindSession: api.pollSession,
  deleteWxPusherChannel: api.deleteChannel,
  updateWxPusherChannel: api.updateChannel,
  testWxPusherChannel: api.testChannel,
  recheckWxPusherTestDelivery: api.recheckTest,
}))

const unboundChannel = {
  configured: true,
  bound: false,
  enabled: false,
  eventTypes: ["pile_available", "credential_expired", "pile_offline"],
  deliveryDisclaimer: "provider notice",
} as const

const boundChannel = {
  ...unboundChannel,
  bound: true,
  enabled: true,
  maskedUid: "••••1234",
  boundAt: "2026-08-24T08:00:00Z",
  eventTypes: ["pile_available", "credential_expired", "pile_offline"],
} as const

beforeEach(() => {
  api.getChannel.mockResolvedValue(unboundChannel)
  api.deleteChannel.mockResolvedValue(undefined)
  api.updateChannel.mockResolvedValue(boundChannel)
  api.testChannel.mockResolvedValue({
    id: "ndl_test",
    status: "pending",
    userState: "queued",
    isTest: true,
    createdAt: "2026-08-24T08:10:00Z",
    updatedAt: "2026-08-24T08:10:00Z",
    message: "等待发送",
  })
  api.recheckTest.mockResolvedValue({
    id: "ndl_test",
    status: "provider_succeeded",
    userState: "processed",
    isTest: true,
    createdAt: "2026-08-24T08:10:00Z",
    acceptedAt: "2026-08-24T08:10:02Z",
    providerSucceededAt: "2026-08-24T08:10:10Z",
    updatedAt: "2026-08-24T08:10:10Z",
    message: "已发送，请检查手机",
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe("WxPusherChannelCard", () => {
  it("shows a clear disabled state when the administrator has not configured WxPusher", async () => {
    api.getChannel.mockResolvedValue({ ...unboundChannel, configured: false })
    render(<WxPusherChannelCard active />)

    expect(await screen.findByText("微信提醒暂未开放")).toBeVisible()
    expect(screen.getByText("通知中心和网页提醒仍可正常使用。")).toBeVisible()
    expect(screen.queryByRole("button", { name: "获取二维码" })).toBeNull()
  })

  it("polls after ten seconds and shows binding success", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const base = Date.now()
    api.createSession.mockResolvedValue({
      id: "wxp_session",
      status: "waiting_scan",
      qrUrl: "https://wxpusher.zjiecode.com/api/qrcode/test",
      expiresAt: new Date(base + 600_000).toISOString(),
      nextPollAt: new Date(base + 10_000).toISOString(),
      message: "等待扫码确认。",
    })
    api.pollSession.mockResolvedValue({
      id: "wxp_session",
      status: "bound",
      expiresAt: new Date(base + 600_000).toISOString(),
      nextPollAt: new Date(base + 20_000).toISOString(),
      completedAt: new Date(base + 10_000).toISOString(),
      message: "微信提醒已绑定。",
    })
    api.getChannel.mockResolvedValueOnce(unboundChannel).mockResolvedValueOnce({
      ...unboundChannel,
      bound: true,
      enabled: true,
      maskedUid: "••••1234",
      boundAt: new Date(base).toISOString(),
    })

    render(<WxPusherChannelCard active />)
    await vi.advanceTimersByTimeAsync(1)
    await user.click(await screen.findByRole("button", { name: "获取二维码" }))
    expect(
      await screen.findByAltText("WxPusher 微信提醒绑定二维码")
    ).toBeVisible()
    expect(api.pollSession).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10_100)
    await waitFor(() => expect(api.pollSession).toHaveBeenCalledTimes(1))
    expect(await screen.findByText("微信提醒已绑定")).toBeVisible()
    await vi.advanceTimersByTimeAsync(20_000)
    expect(api.pollSession).toHaveBeenCalledTimes(1)
  })

  it("stops polling as soon as the QR dialog is closed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const base = Date.now()
    api.createSession.mockResolvedValue({
      id: "wxp_close",
      status: "waiting_scan",
      qrUrl: "https://wxpusher.zjiecode.com/api/qrcode/test",
      expiresAt: new Date(base + 600_000).toISOString(),
      nextPollAt: new Date(base + 10_000).toISOString(),
      message: "等待扫码确认。",
    })

    render(<WxPusherChannelCard active />)
    await vi.advanceTimersByTimeAsync(1)
    await user.click(await screen.findByRole("button", { name: "获取二维码" }))
    await user.click(await screen.findByRole("button", { name: "关闭" }))
    await vi.advanceTimersByTimeAsync(20_000)

    expect(api.pollSession).not.toHaveBeenCalled()
    expect(screen.queryByAltText("WxPusher 微信提醒绑定二维码")).toBeNull()

    await user.click(screen.getByRole("button", { name: "获取二维码" }))
    expect(
      await screen.findByRole("dialog", { name: "绑定微信提醒" })
    ).toBeVisible()
    expect(api.createSession).toHaveBeenCalledTimes(1)
  })

  it("updates the master switch and individual message preferences", async () => {
    const user = userEvent.setup()
    api.getChannel.mockResolvedValue(boundChannel)
    api.updateChannel
      .mockResolvedValueOnce({ ...boundChannel, enabled: false })
      .mockResolvedValueOnce({
        ...boundChannel,
        eventTypes: [...boundChannel.eventTypes, "pile_recovered"],
      })

    const { unmount } = render(<WxPusherChannelCard active />)
    await user.click(await screen.findByRole("switch", { name: "微信提醒" }))
    expect(api.updateChannel).toHaveBeenNthCalledWith(1, { enabled: false })

    api.getChannel.mockResolvedValue(boundChannel)
    unmount()
    render(<WxPusherChannelCard active />)
    await user.click(
      await screen.findByRole("switch", { name: "充电桩恢复连接" })
    )
    expect(api.updateChannel).toHaveBeenNthCalledWith(2, {
      eventTypes: [
        "pile_available",
        "credential_expired",
        "pile_offline",
        "pile_recovered",
      ],
    })
  })

  it("queues a test message and shows its latest delivery state", async () => {
    const user = userEvent.setup()
    api.getChannel.mockResolvedValue(boundChannel)

    render(<WxPusherChannelCard active />)
    await user.click(
      await screen.findByRole("button", { name: "发送测试消息" })
    )

    expect(api.testChannel).toHaveBeenCalledTimes(1)
    expect(await screen.findByText(/最近测试 · 等待发送/)).toBeVisible()
    expect(screen.getByText(/测试消息正在发送/)).toBeVisible()
  })

  it("shows an actionable suggestion for a rejected recipient", async () => {
    api.getChannel.mockResolvedValue({
      ...boundChannel,
      lastDelivery: {
        id: "ndl_failed",
        status: "failed",
        isTest: false,
        updatedAt: "2026-08-24T08:10:00Z",
        message: "发送失败",
        errorCode: "recipient_rejected",
      },
    })

    render(<WxPusherChannelCard active />)

    expect(await screen.findByText(/当前接收账号可能已取消关注/)).toBeVisible()
  })

  it("refreshes an in-flight delivery and stops after it reaches a final state", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    api.getChannel
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "accepted",
          userState: "submitted",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:00Z",
          message: "已提交 WxPusher",
        },
      })
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "provider_succeeded",
          userState: "processed",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:10Z",
          message: "已发送，请检查手机",
        },
      })

    render(<WxPusherChannelCard active />)
    await vi.advanceTimersByTimeAsync(1)
    expect(
      await screen.findByText(/最近测试 · 已发送，请检查手机/)
    ).toBeVisible()

    await vi.advanceTimersByTimeAsync(10_100)
    await waitFor(() => expect(api.getChannel).toHaveBeenCalledTimes(2))
    expect(
      await screen.findByText(/最近测试 · 已发送，请检查手机/)
    ).toBeVisible()

    await vi.advanceTimersByTimeAsync(20_000)
    expect(api.getChannel).toHaveBeenCalledTimes(2)
  })

  it("keeps polling while the provider status remains in flight", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    api.getChannel
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "accepted",
          userState: "submitted",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:00Z",
          message: "已提交 WxPusher",
        },
      })
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "accepted",
          userState: "submitted",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:10Z",
          message: "已提交 WxPusher",
        },
      })
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "provider_succeeded",
          userState: "processed",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:20Z",
          message: "已发送，请检查手机",
        },
      })

    render(<WxPusherChannelCard active />)
    await vi.advanceTimersByTimeAsync(1)
    expect(
      await screen.findByText(/最近测试 · 已发送，请检查手机/)
    ).toBeVisible()

    await vi.advanceTimersByTimeAsync(10_100)
    await waitFor(() => expect(api.getChannel).toHaveBeenCalledTimes(2))
    expect(screen.getByText(/最近测试 · 已发送，请检查手机/)).toBeVisible()

    await vi.advanceTimersByTimeAsync(10_100)
    await waitFor(() => expect(api.getChannel).toHaveBeenCalledTimes(3))
    expect(
      await screen.findByText(/最近测试 · 已发送，请检查手机/)
    ).toBeVisible()
  })

  it("pauses delivery polling while hidden and refreshes when visible again", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const originalVisibilityState = document.visibilityState
    api.getChannel
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "accepted",
          userState: "submitted",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:10:00Z",
          message: "已提交 WxPusher",
        },
      })
      .mockResolvedValueOnce({
        ...boundChannel,
        lastTestDelivery: {
          id: "ndl_pending",
          status: "uncertain",
          userState: "submitted",
          isTest: true,
          createdAt: "2026-08-24T08:09:58Z",
          acceptedAt: "2026-08-24T08:10:00Z",
          updatedAt: "2026-08-24T08:20:00Z",
          message: "已发送，请检查手机",
          errorCode: "ambiguous_result",
        },
      })

    try {
      render(<WxPusherChannelCard active />)
      await vi.advanceTimersByTimeAsync(1)
      expect(
        await screen.findByText(/最近测试 · 已发送，请检查手机/)
      ).toBeVisible()

      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "hidden",
      })
      document.dispatchEvent(new Event("visibilitychange"))
      await vi.advanceTimersByTimeAsync(20_000)
      expect(api.getChannel).toHaveBeenCalledTimes(1)

      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "visible",
      })
      document.dispatchEvent(new Event("visibilitychange"))
      await waitFor(() => expect(api.getChannel).toHaveBeenCalledTimes(2))
      expect(
        await screen.findByText(/最近测试 · 已发送，请检查手机/)
      ).toBeVisible()
      expect(screen.queryByText("查看详情")).toBeNull()
    } finally {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: originalVisibilityState,
      })
    }
  })

  it("rechecks an accepted test without offering a resend", async () => {
    const user = userEvent.setup()
    api.getChannel.mockResolvedValue({
      ...boundChannel,
      lastTestDelivery: {
        id: "ndl_accepted",
        status: "accepted",
        userState: "submitted",
        isTest: true,
        createdAt: "2026-08-24T08:09:58Z",
        acceptedAt: "2026-08-24T08:10:00Z",
        updatedAt: "2026-08-24T08:10:04Z",
        message: "已提交 WxPusher",
      },
    })

    const { container } = render(<WxPusherChannelCard active />)

    expect(
      await screen.findByText(/消息已交给 WxPusher；暂未取得进一步状态/)
    ).toBeVisible()
    expect(
      Array.from(container.querySelectorAll("svg")).some((icon) =>
        icon.classList.contains("motion-safe:animate-spin")
      )
    ).toBe(false)
    expect(screen.getByText(/发送 08\/24 16:10/)).toBeVisible()
    expect(screen.getByText(/最近检查 08\/24 16:10/)).toBeVisible()
    expect(screen.queryByRole("button", { name: "重新发送" })).toBeNull()

    await user.click(screen.getByRole("button", { name: "重新查询状态" }))
    expect(api.recheckTest).toHaveBeenCalledTimes(1)
    expect(
      await screen.findByText(/最近测试 · 已发送，请检查手机/)
    ).toBeVisible()
  })

  it("hides technical codes and only offers resend for an explicit failure", async () => {
    const user = userEvent.setup()
    api.getChannel.mockResolvedValue({
      ...boundChannel,
      lastTestDelivery: {
        id: "ndl_failed_test",
        status: "failed",
        isTest: true,
        createdAt: "2026-08-24T08:10:00Z",
        updatedAt: "2026-08-24T08:10:12Z",
        message: "发送失败",
        errorCode: "provider_unavailable",
      },
    })

    render(<WxPusherChannelCard active />)

    expect(await screen.findByText(/WxPusher 暂时繁忙/)).toBeVisible()
    expect(screen.queryByRole("button", { name: "重新查询状态" })).toBeNull()
    expect(screen.queryByText("查看详情")).toBeNull()
    expect(screen.queryByText(/provider_unavailable/)).toBeNull()

    await user.click(screen.getByRole("button", { name: "重新发送" }))
    expect(api.testChannel).toHaveBeenCalledTimes(1)
  })
})
