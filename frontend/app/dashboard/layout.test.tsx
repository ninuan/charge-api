import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import DashboardLayout from "./layout"

vi.mock("@/components/workbench/dashboard-session", () => ({
  DashboardSession: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dashboard-session">{children}</div>
  ),
}))

vi.mock("@/lib/dashboard-context", () => ({
  DashboardProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dashboard-provider">{children}</div>
  ),
}))

vi.mock("@/lib/watch-context", () => ({
  WatchProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="watch-provider">{children}</div>
  ),
}))

vi.mock("@/lib/notification-context", () => ({
  NotificationProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="notification-provider">{children}</div>
  ),
}))

describe("DashboardLayout", () => {
  it("provides dashboard and watch state only within the dashboard route", () => {
    render(
      <DashboardLayout>
        <p>看板内容</p>
      </DashboardLayout>
    )

    expect(screen.getByTestId("dashboard-provider")).toHaveTextContent(
      "看板内容"
    )
    expect(screen.getByTestId("watch-provider")).toHaveTextContent("看板内容")
    expect(screen.getByTestId("notification-provider")).toHaveTextContent(
      "看板内容"
    )
    expect(screen.getByTestId("dashboard-session")).toHaveTextContent(
      "看板内容"
    )
  })
})
