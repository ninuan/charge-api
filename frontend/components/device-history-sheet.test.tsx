import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import {
  DeviceHistorySheet,
  HistoryHeatmap,
} from "@/components/device-history-sheet"
import type {
  DeviceHistoryResponse,
  PortHistoryMetrics,
  PortHistoryResponse,
} from "@/lib/api/generated"
import { historyApi } from "@/lib/history-api"
import type { Pile } from "@/lib/types"

vi.mock("@/lib/history-api", () => ({
  historyApi: {
    device: vi.fn(),
    port: vi.fn(),
  },
}))

const metrics: PortHistoryMetrics = {
  observedSeconds: 432000,
  gapSeconds: 172800,
  idleSeconds: 302400,
  inUseSeconds: 129600,
  offlineSeconds: 0,
  occupancyPercent: 30,
  completedSessions: 6,
  averageSessionSeconds: 3600,
  sampleState: "partial",
}

const daily = Array.from({ length: 7 }, (_, index) => ({
  date: `2026-07-${String(9 + index).padStart(2, "0")}`,
  metrics: { ...metrics, occupancyPercent: 20 + index * 5 },
}))

const heatmap = Array.from({ length: 7 * 24 }, (_, index) => ({
  weekday: Math.floor(index / 24) + 1,
  hour: index % 24,
  idleSeconds: 7200,
  inUseSeconds: 3600,
  offlineSeconds: 0,
  occupancyPercent: 33.3,
  sampleDates: 4,
  sampleSufficient: true,
}))

const deviceHistory: DeviceHistoryResponse = {
  device: {
    id: "2601201412385560001",
    number: "61034278",
    name: "松园 3 号楼",
    address: "北门",
  },
  window: {
    range: "7d",
    timezone: "Asia/Shanghai",
    start: "2026-07-09T09:00:00Z",
    end: "2026-07-16T09:00:00Z",
  },
  metrics,
  daily,
  heatmap,
  ports: [
    { portId: 1, currentStatus: "idle", metrics },
    { portId: 2, currentStatus: "in_use", metrics },
  ],
  busiestHours: [{ weekday: 1, hour: 9, occupancyPercent: 78, sampleDates: 4 }],
  quietSuggestion: {
    weekday: 3,
    hour: 14,
    occupancyPercent: 12,
    sampleDates: 4,
  },
  historyStartedAt: "2026-07-09T09:00:00Z",
  historyNotice: "历史从启用记录后开始。",
}

function portHistory(portId: number): PortHistoryResponse {
  return {
    device: deviceHistory.device,
    portId,
    currentStatus: portId === 1 ? "idle" : "in_use",
    window: deviceHistory.window,
    metrics,
    daily,
    timeline: [
      {
        portId,
        fromStatus: "idle",
        toStatus: "in_use",
        changedAt: "2026-07-15T08:00:00Z",
        usedSeconds: 0,
      },
      {
        portId,
        fromStatus: "in_use",
        toStatus: "idle",
        changedAt: "2026-07-15T09:00:00Z",
        usedSeconds: 3600,
      },
    ],
    timelineTruncated: false,
    historyStartedAt: "2026-07-09T09:00:00Z",
    historyNotice: deviceHistory.historyNotice,
  }
}

const pile: Pile = {
  id: deviceHistory.device.id,
  number: deviceHistory.device.number,
  name: deviceHistory.device.name,
  address: deviceHistory.device.address,
  status: "在线",
  openNum: 2,
  online: true,
  createdAt: "2026-07-09T09:00:00Z",
  updatedAt: "2026-07-16T09:00:00Z",
  source: "remote",
  ports: [
    {
      id: 1,
      status: "idle",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "2026-07-16T09:00:00Z",
      sessionMin: 0,
      usedSeconds: 0,
    },
    {
      id: 2,
      status: "in_use",
      powerKw: 6.6,
      energyKwh: 2.4,
      updatedAt: "2026-07-16T09:00:00Z",
      sessionMin: 20,
      usedSeconds: 1200,
    },
  ],
  usedPortIds: [2],
  sortOrder: 0,
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("DeviceHistorySheet", () => {
  it("loads the summary and initial port together", async () => {
    vi.mocked(historyApi.device).mockResolvedValue(deviceHistory)
    vi.mocked(historyApi.port).mockResolvedValue(portHistory(1))

    render(<DeviceHistorySheet pile={pile} open onOpenChange={vi.fn()} />)

    expect(await screen.findByText("最近 7 天占用率")).toBeInTheDocument()
    expect(screen.getAllByText("30.0%")).not.toHaveLength(0)
    expect(screen.getByText("周一 09:00")).toBeInTheDocument()
    expect(screen.getByText("历史从启用记录后开始。")).toBeInTheDocument()
    expect(
      screen.getByRole("group", { name: "星期与小时占用率热力图" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("list", { name: "1 号口状态时间线" })
    ).toBeInTheDocument()
    expect(historyApi.device).toHaveBeenCalledTimes(1)
    expect(historyApi.port).toHaveBeenCalledWith(
      pile.id,
      1,
      expect.objectContaining({ range: "7d" }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it("switches ports through accessible tabs", async () => {
    const user = userEvent.setup()
    vi.mocked(historyApi.device).mockResolvedValue(deviceHistory)
    vi.mocked(historyApi.port).mockImplementation(async (_, portId) =>
      portHistory(portId)
    )

    render(<DeviceHistorySheet pile={pile} open onOpenChange={vi.fn()} />)

    await screen.findByRole("list", { name: "1 号口状态时间线" })
    await user.click(screen.getByRole("tab", { name: "02 号" }))

    expect(
      await screen.findByRole("list", { name: "2 号口状态时间线" })
    ).toBeInTheDocument()
    expect(historyApi.port).toHaveBeenLastCalledWith(
      pile.id,
      2,
      expect.objectContaining({ range: "7d" }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it("offers a retry when the device history request fails", async () => {
    const user = userEvent.setup()
    vi.mocked(historyApi.device)
      .mockRejectedValueOnce(new Error("历史服务暂时不可用"))
      .mockResolvedValueOnce(deviceHistory)
    vi.mocked(historyApi.port).mockResolvedValue(portHistory(1))

    render(<DeviceHistorySheet pile={pile} open onOpenChange={vi.fn()} />)

    expect(await screen.findByText("历史服务暂时不可用")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "重试" }))

    await waitFor(() =>
      expect(screen.getByText("最近 7 天占用率")).toBeInTheDocument()
    )
    expect(historyApi.device).toHaveBeenCalledTimes(2)
  })

  it("explains an empty history without presenting it as zero occupancy", async () => {
    const emptyMetrics: PortHistoryMetrics = {
      ...metrics,
      observedSeconds: 0,
      gapSeconds: 604800,
      idleSeconds: 0,
      inUseSeconds: 0,
      occupancyPercent: null,
      completedSessions: 0,
      averageSessionSeconds: null,
      sampleState: "no_data",
    }
    vi.mocked(historyApi.device).mockResolvedValue({
      ...deviceHistory,
      metrics: emptyMetrics,
      daily: daily.map((point) => ({ ...point, metrics: emptyMetrics })),
      heatmap: heatmap.map((cell) => ({
        ...cell,
        occupancyPercent: null,
        sampleDates: 0,
        sampleSufficient: false,
      })),
      busiestHours: [],
      quietSuggestion: undefined,
      historyStartedAt: undefined,
    })
    vi.mocked(historyApi.port).mockResolvedValue({
      ...portHistory(1),
      metrics: emptyMetrics,
      timeline: [],
      historyStartedAt: undefined,
    })

    render(<DeviceHistorySheet pile={pile} open onOpenChange={vi.fn()} />)

    expect(await screen.findByText("历史数据正在积累")).toBeInTheDocument()
    expect(screen.getAllByText("数据不足")).not.toHaveLength(0)
    expect(screen.queryByText("0.0%")).not.toBeInTheDocument()
    expect(screen.getByText("暂时没有状态变化")).toBeInTheDocument()
  })

  it("retries a failed port without reloading the device summary", async () => {
    const user = userEvent.setup()
    vi.mocked(historyApi.device).mockResolvedValue(deviceHistory)
    vi.mocked(historyApi.port)
      .mockRejectedValueOnce(new Error("端口服务暂时不可用"))
      .mockResolvedValueOnce(portHistory(1))

    render(<DeviceHistorySheet pile={pile} open onOpenChange={vi.fn()} />)

    expect(await screen.findByText("端口服务暂时不可用")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "重试当前端口" }))

    expect(
      await screen.findByRole("list", { name: "1 号口状态时间线" })
    ).toBeInTheDocument()
    expect(historyApi.port).toHaveBeenCalledTimes(2)
    expect(historyApi.device).toHaveBeenCalledTimes(1)
  })
})

describe("HistoryHeatmap", () => {
  it("remounts only cells whose data changed", () => {
    const { rerender } = render(<HistoryHeatmap cells={heatmap} />)
    const changedButton = screen.getByRole("button", { name: /周一 00:00/ })
    const stableButton = screen.getByRole("button", { name: /周一 01:00/ })
    const changedCell = changedButton.firstElementChild
    const stableCell = stableButton.firstElementChild

    rerender(
      <HistoryHeatmap
        cells={heatmap.map((cell, index) =>
          index === 0 ? { ...cell, occupancyPercent: 64 } : cell
        )}
      />
    )

    expect(changedButton.firstElementChild).not.toBe(changedCell)
    expect(stableButton.firstElementChild).toBe(stableCell)
    expect(changedButton.firstElementChild).toHaveClass("history-heatmap-cell")
  })
})
