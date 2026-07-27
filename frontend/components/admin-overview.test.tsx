import { cleanup, render, screen } from "@testing-library/react"
import { ActivityIcon } from "lucide-react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminOverview } from "@/components/admin-overview"
import { MetricCard } from "@/components/metric-card"
import type { AdminStats, MetricPoint } from "@/lib/types"

const point = (
  time: string,
  remote: number,
  activeUsers: number
): MetricPoint => ({
  time,
  requests: remote,
  remote,
  cacheHits: 0,
  remoteOk: remote,
  remoteFailed: 0,
  cookieErrors: 0,
  activeUsers,
})

const stats: AdminStats = {
  overview: {
    openIssues: 0,
    remoteSuccessRate: 100,
    activeUsers: 2,
    managedDevices: 3,
    offlinePorts: 0,
  },
  users: [],
  hourly: [point("10:00", 2, 1), point("11:00", 3, 2)],
  daily: [point("2026-07-26", 4, 2), point("2026-07-27", 5, 3)],
  exceptions: [],
}

describe("AdminOverview", () => {
  afterEach(cleanup)

  it("shows explicit metric units and accessible trend ranges", () => {
    render(<AdminOverview stats={stats} onUser={vi.fn()} />)

    expect(screen.getByLabelText("远端成功率 指标")).toHaveTextContent("100%")
    expect(
      screen.getByRole("img", {
        name: "过去 2 小时远端请求趋势，共 5 次，成功率 100%",
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("img", {
        name: "过去 2 天活跃用户趋势，单日峰值 3 人",
      })
    ).toBeInTheDocument()
    expect(screen.getByText("峰值 3 人")).toBeInTheDocument()
    expect(screen.getByText("0 条")).toBeInTheDocument()
  })
})

describe("MetricCard", () => {
  afterEach(cleanup)

  it("renders a suffix next to the value", () => {
    render(
      <MetricCard
        compact
        label="成功率"
        value={100}
        suffix="%"
        detail="近期"
        icon={ActivityIcon}
      />
    )

    expect(screen.getByLabelText("成功率 指标")).toHaveTextContent("100%")
  })
})
