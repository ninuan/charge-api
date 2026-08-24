import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { WxPusherChannelCard } from "@/components/wxpusher-channel-card"

const api = vi.hoisted(() => ({
  getChannel: vi.fn(),
  createSession: vi.fn(),
  pollSession: vi.fn(),
  deleteChannel: vi.fn(),
}))

vi.mock("@/lib/wxpusher-api", () => ({
  getWxPusherChannel: api.getChannel,
  createWxPusherBindSession: api.createSession,
  pollWxPusherBindSession: api.pollSession,
  deleteWxPusherChannel: api.deleteChannel,
}))

const unboundChannel = {
  configured: true,
  bound: false,
  enabled: false,
  eventTypes: ["pile_available", "credential_expired", "pile_offline"],
  deliveryDisclaimer: "provider notice",
} as const

beforeEach(() => {
  api.getChannel.mockResolvedValue(unboundChannel)
  api.deleteChannel.mockResolvedValue(undefined)
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
    expect(screen.getByText("站内通知和浏览器提醒仍可正常使用。")).toBeVisible()
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
})
