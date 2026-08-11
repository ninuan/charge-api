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

describe("DashboardProvider", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("loads the snapshot with the existing credentialed request", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify(snapshot), { status: 200 })
      )
    vi.stubGlobal("fetch", fetchMock)
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    await act(() => result.current.fetchSnapshot())

    expect(fetchMock).toHaveBeenCalledWith("/api/piles", {
      credentials: "include",
      signal: expect.any(AbortSignal),
    })
    expect(result.current.snapshot.statistics.pileCount).toBe(2)
  })

  it("uses the add-pile fallback instead of an internal server error", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ error: "dial internal-yyb:8443 with token=secret" }),
          { status: 502 }
        )
      )
    vi.stubGlobal("fetch", fetchMock)
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    await expect(
      result.current.addPile({
        id: "61034278",
        name: "充电桩 61034278",
        number: "61034278",
        openNum: 10,
        status: "在线",
        address: "松园 3 号楼",
      })
    ).rejects.toThrow("添加充电桩失败，请检查桩号后重试。")
  })

  it("marks the stream connected as soon as EventSource opens", () => {
    class MockEventSource {
      static latest: MockEventSource | null = null
      onerror: ((event: Event) => void) | null = null
      onopen: ((event: Event) => void) | null = null
      constructor() {
        MockEventSource.latest = this
      }
      addEventListener = vi.fn()
      close = vi.fn()
    }
    vi.stubGlobal("EventSource", MockEventSource)
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    act(() => result.current.connectStream())
    act(() => MockEventSource.latest?.onopen?.(new Event("open")))

    expect(result.current.streamState).toBe("connected")
  })

  it("dispatches notification events and reports stream reconnects", () => {
    class MockEventSource {
      static OPEN = 1
      static latest: MockEventSource | null = null
      readyState = MockEventSource.OPEN
      onerror: ((event: Event) => void) | null = null
      onopen: ((event: Event) => void) | null = null
      listeners = new Map<string, (event: Event) => void>()
      constructor() {
        MockEventSource.latest = this
      }
      addEventListener = vi.fn(
        (name: string, listener: EventListenerOrEventListenerObject) => {
          this.listeners.set(name, listener as (event: Event) => void)
        }
      )
      close = vi.fn()
    }
    vi.stubGlobal("EventSource", MockEventSource)
    const notificationListener = vi.fn()
    const openListener = vi.fn()
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    act(() => {
      result.current.subscribeNotifications(notificationListener)
      result.current.subscribeStreamOpen(openListener)
      result.current.connectStream()
    })
    act(() => MockEventSource.latest?.onopen?.(new Event("open")))
    act(() => {
      MockEventSource.latest?.listeners.get("notification")?.(
        new MessageEvent("notification", {
          data: JSON.stringify({
            id: "notice-1",
            userId: "user-1",
            type: "pile_available",
            severity: "info",
            title: "有空闲口",
            message: "松园充电桩出现空闲口",
            createdAt: "2026-08-11T09:00:00Z",
          }),
        })
      )
    })

    expect(openListener).toHaveBeenCalledTimes(1)
    expect(notificationListener).toHaveBeenCalledWith(
      expect.objectContaining({ id: "notice-1", type: "pile_available" })
    )
  })
})
