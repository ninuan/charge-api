import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DashboardProvider, useDashboard } from "@/lib/dashboard-context"

const snapshot = {
  piles: [],
  updatedAt: "2026-07-15T00:00:00Z",
  statistics: {
    pileCount: 2,
    portCount: 4,
    inUsePortCount: 1,
    idlePortCount: 2,
    offlinePorts: 1,
  },
  refresh: {
    minIntervalSeconds: 30,
    attemptedDevices: 1,
    successfulDevices: 1,
    failedDevices: 0,
    skippedDevices: 0,
    cached: false,
    partial: false,
  },
}

describe("DashboardProvider mutations", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("returns the exact refresh response used to update the dashboard", async () => {
    const refreshed = {
      ...snapshot,
      updatedAt: "2026-07-15T00:01:00Z",
      refresh: { ...snapshot.refresh, cached: true, message: "已返回缓存数据" },
    }
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(refreshed), { status: 200 })
      )
    )
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    let response
    await act(async () => {
      response = await result.current.refreshFromCapture()
    })

    expect(response).toEqual(refreshed)
    expect(result.current.snapshot.refresh.message).toBe("已返回缓存数据")
  })

  it("submits the full pile order and applies the returned snapshot", async () => {
    const reordered = {
      ...snapshot,
      updatedAt: "2026-07-15T00:02:00Z",
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(reordered), { status: 200 })
      )
    vi.stubGlobal("fetch", fetchMock)
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    await act(() => result.current.reorderPiles(["pile-2", "pile-1"]))

    expect(fetchMock).toHaveBeenCalledWith("/api/piles/order", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids: ["pile-2", "pile-1"] }),
      credentials: "include",
      signal: expect.any(AbortSignal),
    })
  })
})
