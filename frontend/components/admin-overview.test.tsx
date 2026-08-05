import { cleanup, render, screen } from "@testing-library/react"
import { ActivityIcon } from "lucide-react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminOverview } from "@/components/admin-overview"
import { MetricCard } from "@/components/metric-card"
import type { AdminTrendsResponse } from "@/lib/api/generated"
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

const trends: AdminTrendsResponse = {
  window: {
    range: "24h",
    timezone: "Asia/Shanghai",
    bucketUnit: "hour",
    start: "2026-07-26T10:00:00Z",
    end: "2026-07-27T10:00:00Z",
  },
  summary: {
    requests: 5,
    remoteAttempts: 5,
    remoteSuccesses: 5,
    remoteFailures: 0,
    remoteSuccessRate: 100,
    activeUsers: 2,
    offlinePorts: 0,
  },
  points: [
    {
      start: "2026-07-27T08:00:00Z",
      end: "2026-07-27T09:00:00Z",
      requests: 2,
      remoteAttempts: 2,
      remoteSuccesses: 2,
      remoteFailures: 0,
      remoteSuccessRate: 100,
      activeUsers: 1,
      offlinePorts: 0,
    },
    {
      start: "2026-07-27T09:00:00Z",
      end: "2026-07-27T10:00:00Z",
      requests: 3,
      remoteAttempts: 3,
      remoteSuccesses: 3,
      remoteFailures: 0,
      remoteSuccessRate: 100,
      activeUsers: 2,
      offlinePorts: 0,
    },
  ],
  updatedAt: "2026-07-27T10:00:00Z",
}

describe("AdminOverview", () => {
  afterEach(cleanup)

  it("shows explicit metric units and the unified trend panel", () => {
    render(
      <AdminOverview
        stats={stats}
        trends={trends}
        trendRange="24h"
        trendLoading={false}
        trendError={null}
        onTrendRangeChange={vi.fn()}
        onTrendReload={vi.fn()}
        onUser={vi.fn()}
      />
    )

    expect(screen.getByLabelText("远端成功率 指标")).toHaveTextContent("100%")
    expect(
      screen.getByRole("group", {
        name: "24 小时远端成功率趋势",
      })
    ).toBeInTheDocument()
    expect(screen.getByText("100.0%")).toBeInTheDocument()
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
