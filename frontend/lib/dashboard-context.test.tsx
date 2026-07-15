import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DashboardProvider, useDashboard } from "@/lib/dashboard-context"

const snapshot = {
  piles: [],
  updatedAt: "2026-07-15T00:00:00Z",
  statistics: { pileCount: 2, portCount: 4, inUsePortCount: 1, idlePortCount: 2, offlinePorts: 1 },
  refresh: { minIntervalSeconds: 30, attemptedDevices: 1, successfulDevices: 1, failedDevices: 0, skippedDevices: 0, cached: false, partial: false },
}

describe("DashboardProvider", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("loads the snapshot with the existing credentialed request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(snapshot), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const { result } = renderHook(() => useDashboard(), { wrapper: DashboardProvider })

    await act(() => result.current.fetchSnapshot())

    expect(fetchMock).toHaveBeenCalledWith("/api/piles", { credentials: "include" })
    expect(result.current.snapshot.statistics.pileCount).toBe(2)
  })
})
