import { beforeEach, describe, expect, it, vi } from "vitest"

const toastMock = vi.hoisted(() => ({
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  dismiss: vi.fn(),
}))

vi.mock("sonner", () => ({ toast: toastMock }))

import { feedbackMessage, notify } from "@/lib/feedback"

describe("feedback", () => {
  beforeEach(() => vi.clearAllMocks())

  it("keeps the result as the title and supporting detail as the description", () => {
    notify.error(new Error("网络连接超时"), {
      title: "刷新失败",
      id: "dashboard-refresh",
    })

    expect(toastMock.error).toHaveBeenCalledWith("刷新失败", {
      duration: 6_500,
      id: "dashboard-refresh",
      description: "网络连接超时",
    })
  })

  it("uses a safe fallback for unknown failures", () => {
    expect(feedbackMessage(null)).toBe("操作没有完成，请稍后重试")
    notify.error(null, { title: "保存失败", fallback: "请稍后再试" })

    expect(toastMock.error).toHaveBeenCalledWith("保存失败", {
      duration: 6_500,
      description: "请稍后再试",
    })
  })

  it("allows a stable id and retry action to replace repeated feedback", () => {
    const retry = vi.fn()
    notify.error("暂时无法读取数据", {
      title: "加载失败",
      id: "admin-overview-load",
      action: { label: "重试", onClick: retry },
    })

    expect(toastMock.error).toHaveBeenCalledWith(
      "加载失败",
      expect.objectContaining({
        id: "admin-overview-load",
        action: { label: "重试", onClick: retry },
      })
    )
  })
})
