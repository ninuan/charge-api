import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import DashboardLayout from "./layout"

vi.mock("@/lib/dashboard-context", () => ({
  DashboardProvider: ({ children }: { children: React.ReactNode }) => <div data-testid="dashboard-provider">{children}</div>,
}))

describe("DashboardLayout", () => {
  it("provides dashboard state only within the dashboard route", () => {
    render(<DashboardLayout><p>看板内容</p></DashboardLayout>)

    expect(screen.getByTestId("dashboard-provider")).toHaveTextContent("看板内容")
  })
})
