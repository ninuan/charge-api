import { act, cleanup, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DashboardProvider, useDashboard } from "@/lib/dashboard-context"
import type { Pile } from "@/lib/types"

const samplePile: Pile = {
  id: "2601201412385560001",
  number: "61034278",
  name: "松园 3 号楼",
  status: "在线",
  address: "北门",
  openNum: 2,
  online: true,
  createdAt: "2026-07-15T00:00:00Z",
  updatedAt: "2026-07-15T00:00:00Z",
  source: "remote",
  ports: [
    {
      id: 1,
      status: "idle",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "2026-07-15T00:00:00Z",
      sessionMin: 0,
      usedSeconds: 0,
    },
    {
      id: 2,
      status: "in_use",
      powerKw: 0.3,
      energyKwh: 0.5,
      updatedAt: "2026-07-15T00:00:00Z",
      sessionMin: 15,
      usedSeconds: 900,
    },
  ],
  usedPortIds: [2],
  sortOrder: 0,
}

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
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

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

  it("immediately removes a deleted pile and updates counters even when SSE is open without pushing", async () => {
    class OpenEventSource {
      static OPEN = 1
      static CLOSED = 2
      readyState = 1
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      addEventListener() {}
      close() {
        this.readyState = 2
      }
    }
    vi.stubGlobal("EventSource", OpenEventSource)
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ...snapshot,
            piles: [samplePile],
            statistics: {
              pileCount: 1,
              portCount: 2,
              inUsePortCount: 1,
              idlePortCount: 1,
              offlinePorts: 0,
            },
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", fetchMock)

    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    await act(() => result.current.fetchSnapshot())
    act(() => result.current.connectStream())
    expect(result.current.snapshot.piles).toHaveLength(1)

    await act(() => result.current.deletePile(samplePile.id))

    expect(result.current.snapshot.piles).toHaveLength(0)
    expect(result.current.snapshot.statistics).toEqual({
      pileCount: 0,
      portCount: 0,
      inUsePortCount: 0,
      idlePortCount: 0,
      offlinePorts: 0,
    })
  })

  it("immediately reflects updated pile details even when SSE is open without pushing", async () => {
    class OpenEventSource {
      static OPEN = 1
      static CLOSED = 2
      readyState = 1
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      addEventListener() {}
      close() {
        this.readyState = 2
      }
    }
    vi.stubGlobal("EventSource", OpenEventSource)
    const updatedPile: Pile = {
      ...samplePile,
      name: "松园 3 号楼东侧",
      address: "东门车棚",
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ...snapshot,
            piles: [samplePile],
          }),
          { status: 200 }
        )
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(updatedPile), { status: 200 })
      )
    vi.stubGlobal("fetch", fetchMock)

    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    await act(() => result.current.fetchSnapshot())
    act(() => result.current.connectStream())

    await act(() =>
      result.current.updatePile(samplePile.id, {
        name: "松园 3 号楼东侧",
        address: "东门车棚",
        sortOrder: 0,
      })
    )

    expect(result.current.snapshot.piles[0]?.name).toBe("松园 3 号楼东侧")
    expect(result.current.snapshot.piles[0]?.address).toBe("东门车棚")
  })
})
