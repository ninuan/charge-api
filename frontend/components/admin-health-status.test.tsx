import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import { AdminHealthStatus } from "@/components/admin-health-status"
import type { AdminHealth } from "@/lib/types"

const health: AdminHealth = {
  checkedAt: "2026-07-27T08:00:00Z",
  charge: { state: "healthy", message: "服务正常" },
  database: { state: "healthy", message: "存储正常" },
  yyb: { state: "degraded", message: "扫码服务连接异常" },
}

describe("AdminHealthStatus", () => {
  afterEach(cleanup)

  it("summarizes health in the header and reveals service details", async () => {
    const user = userEvent.setup()
    render(<AdminHealthStatus health={health} />)

    const trigger = screen.getByRole("button", {
      name: "服务健康：2/3 正常，查看详情",
    })
    expect(trigger).toHaveTextContent("2/3 正常")

    await user.click(trigger)

    expect(
      screen.getByRole("heading", { name: "服务健康" })
    ).toBeInTheDocument()
    expect(screen.getByText("扫码服务连接异常")).toBeInTheDocument()
    expect(screen.getByText("异常")).toBeInTheDocument()
  })

  it("shows a stable checking state before health data arrives", () => {
    render(<AdminHealthStatus health={null} />)

    expect(
      screen.getByRole("button", {
        name: "服务健康：检查中，查看详情",
      })
    ).toBeInTheDocument()
  })
})
