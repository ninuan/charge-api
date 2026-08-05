import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AdminTrendPanel } from "@/components/admin-trend-panel"
import { adminApi } from "@/lib/admin-api"
import type { AdminTrendsResponse } from "@/lib/api/generated"

const trends: AdminTrendsResponse = {
  window: {
    range: "24h",
    timezone: "Asia/Shanghai",
    bucketUnit: "hour",
    start: "2026-08-04T10:00:00Z",
    end: "2026-08-05T10:00:00Z",
  },
  summary: {
    requests: 120,
    remoteAttempts: 20,
    remoteSuccesses: 15,
    remoteFailures: 5,
    remoteSuccessRate: 75,
    activeUsers: 8,
    offlinePorts: 2,
  },
  points: [
    {
      start: "2026-08-05T08:00:00Z",
      end: "2026-08-05T09:00:00Z",
      requests: 50,
      remoteAttempts: 8,
      remoteSuccesses: 6,
      remoteFailures: 2,
      remoteSuccessRate: 75,
      activeUsers: 5,
      offlinePorts: 1,
    },
    {
      start: "2026-08-05T09:00:00Z",
      end: "2026-08-05T10:00:00Z",
      requests: 70,
      remoteAttempts: 12,
      remoteSuccesses: 9,
      remoteFailures: 3,
      remoteSuccessRate: 75,
      activeUsers: 6,
      offlinePorts: 2,
    },
  ],
  updatedAt: "2026-08-05T10:00:00Z",
}

const defaultProps = {
  trends,
  requestedRange: "24h" as const,
  loading: false,
  error: null,
  onRangeChange: vi.fn(),
  onReload: vi.fn(),
}

describe("AdminTrendPanel", () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    defaultProps.onRangeChange.mockReset()
    defaultProps.onReload.mockReset()
  })

  it("switches ranges and metrics while exposing exact values to keyboard focus", async () => {
    render(<AdminTrendPanel {...defaultProps} />)

    expect(screen.getByText("75.0%")).toBeInTheDocument()
    expect(
      screen.getByRole("group", { name: "24 小时远端成功率趋势" })
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole("tab", { name: "7 天" }))
    expect(defaultProps.onRangeChange).toHaveBeenCalledWith("7d")

    await userEvent.click(screen.getByRole("tab", { name: "请求量" }))
    expect(screen.getByText("120 次")).toBeInTheDocument()
    const firstPoint = screen.getByRole("button", {
      name: /08\/05 16:00 至 08\/05 17:00，请求量 50 次/,
    })
    fireEvent.focus(firstPoint)
    fireEvent.mouseEnter(
      screen.getByRole("button", {
        name: /08\/05 17:00 至 08\/05 18:00，请求量 70 次/,
      })
    )
    expect(
      screen.getByText(/08\/05 16:00 至 08\/05 17:00，请求量 50 次/, {
        selector: "p",
      })
    ).toBeInTheDocument()
  })

  it("downloads the CSV returned for the selected range", async () => {
    const csv = new Blob(["a,b\n1,2"], { type: "text/csv" })
    vi.spyOn(adminApi, "trendsCSV").mockResolvedValue({
      blob: csv,
      filename: "charge-trends-24h-2026-08-05.csv",
    })
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:trend-csv")
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined)
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined)

    render(<AdminTrendPanel {...defaultProps} />)
    await userEvent.click(screen.getByRole("button", { name: "导出 CSV" }))

    expect(adminApi.trendsCSV).toHaveBeenCalledWith("24h", "Asia/Shanghai")
    expect(createObjectURL).toHaveBeenCalledWith(csv)
    expect(click).toHaveBeenCalledOnce()
  })

  it("shows a retry action when the trend request fails", async () => {
    render(
      <AdminTrendPanel
        {...defaultProps}
        trends={null}
        error="运营趋势暂时不可用"
      />
    )

    expect(screen.getByRole("alert")).toHaveTextContent("趋势加载失败")
    await userEvent.click(screen.getByRole("button", { name: "重新加载" }))
    expect(defaultProps.onReload).toHaveBeenCalledOnce()
  })
})
