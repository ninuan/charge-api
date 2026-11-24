import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { UsageGuideDialog } from "@/components/usage-guide-dialog"

vi.mock("@/lib/auth-context", () => ({
  useAuth: () => ({
    currentUser: { id: "u1", role: "user", usageGuideAckAt: undefined },
    acknowledgeUsageGuide: vi.fn(),
  }),
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
})
