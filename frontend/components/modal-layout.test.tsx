import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { UsageGuideDialog } from "@/components/usage-guide-dialog"
import { YybLoginDialog } from "@/components/yyb-login-dialog"

vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({
    currentUser: { id: "u1", role: "user", usageGuideAckAt: undefined },
    acknowledgeUsageGuide: vi.fn(),
  }),
}))
vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({ updateCookie: vi.fn() }),
}))

describe("dashboard modal layouts", () => {
  it("keeps the usage guide navigation visible while the guide body scrolls", async () => {
    render(<UsageGuideDialog />)

    const navigation = await screen.findByText("操作路径")
    expect(navigation.closest("aside")).toHaveClass("sticky", "top-0")
    expect(
      screen.getByRole("link", { name: /使用空闲提醒/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { name: "使用空闲提醒" })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/账户与设置 → 通知设置.*允许通知，并保持网页打开/)
    ).toBeInTheDocument()
  })

  it("keeps QR-login content scrollable on short screens", async () => {
    const user = userEvent.setup()
    render(<YybLoginDialog />)

    await user.click(screen.getByRole("button", { name: "扫码登录" }))
    await waitFor(() =>
      expect(screen.getByText("绑定平台微信")).toBeInTheDocument()
    )
    expect(
      screen.getByText("绑定平台微信").closest("[data-slot=dialog-content]")
    ).toHaveClass("overflow-y-auto", "sm:max-w-lg")
  })
})
