import { act, cleanup, renderHook } from "@testing-library/react"
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
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

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

  it("isolates throwing notification listeners and resets stuck CONNECTING stream on online", () => {
    class MockEventSource {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSED = 2
      static instances: MockEventSource[] = []
      readyState = MockEventSource.CONNECTING
      onerror: ((event: Event) => void) | null = null
      onopen: ((event: Event) => void) | null = null
      listeners = new Map<string, (event: Event) => void>()
      constructor() {
        MockEventSource.instances.push(this)
      }
      addEventListener = vi.fn(
        (name: string, listener: EventListenerOrEventListenerObject) => {
          this.listeners.set(name, listener as (event: Event) => void)
        }
      )
      close = vi.fn()
    }
    vi.stubGlobal("EventSource", MockEventSource)
    const brokenListener = vi.fn(() => {
      throw new TypeError("Illegal constructor")
    })
    const healthyListener = vi.fn()
    const { result } = renderHook(() => useDashboard(), {
      wrapper: DashboardProvider,
    })

    act(() => {
      result.current.subscribeNotifications(brokenListener)
      result.current.subscribeNotifications(healthyListener)
      result.current.connectStream()
    })
    expect(MockEventSource.instances).toHaveLength(1)

    // Stuck in CONNECTING: online event closes and opens a fresh EventSource.
    act(() => {
      window.dispatchEvent(new Event("online"))
    })
    expect(MockEventSource.instances[0].close).toHaveBeenCalledTimes(1)
    expect(MockEventSource.instances).toHaveLength(2)

    // Once OPEN, online event leaves the healthy stream untouched.
    act(() => {
      MockEventSource.instances[1].readyState = MockEventSource.OPEN
      MockEventSource.instances[1].onopen?.(new Event("open"))
      window.dispatchEvent(new Event("online"))
    })
    expect(MockEventSource.instances).toHaveLength(2)

    // A throwing subscriber must not break other subscribers or flip streamState to error.
    act(() => {
      MockEventSource.instances[1].listeners.get("notification")?.(
        new MessageEvent("notification", {
          data: JSON.stringify({
            id: "notice-2",
            userId: "user-1",
            type: "pile_available",
            severity: "info",
            title: "有空闲口",
            message: "北门桩出现空闲口",
            createdAt: "2026-08-11T09:05:00Z",
          }),
        })
      )
    })
    expect(brokenListener).toHaveBeenCalledTimes(1)
    expect(healthyListener).toHaveBeenCalledTimes(1)
    expect(result.current.streamState).toBe("connected")
  })

  it("reconnects with backoff after EventSource.CLOSED and cancels on disconnect", () => {
    vi.useFakeTimers()
    try {
      class MockEventSource {
        static CLOSED = 2
        static instances: MockEventSource[] = []
        readyState = 0
        onerror: ((event: Event) => void) | null = null
        onopen: ((event: Event) => void) | null = null
        constructor() {
          MockEventSource.instances.push(this)
        }
        addEventListener = vi.fn()
        close = vi.fn()
      }
      vi.stubGlobal("EventSource", MockEventSource)
      const { result } = renderHook(() => useDashboard(), {
        wrapper: DashboardProvider,
      })

      act(() => result.current.connectStream())
      expect(MockEventSource.instances).toHaveLength(1)

      act(() => {
        MockEventSource.instances[0].readyState = MockEventSource.CLOSED
        MockEventSource.instances[0].onerror?.(new Event("error"))
      })
      expect(result.current.streamState).toBe("error")

      act(() => {
        vi.advanceTimersByTime(1999)
      })
      expect(MockEventSource.instances).toHaveLength(1)

      act(() => {
        vi.advanceTimersByTime(1)
      })
      expect(MockEventSource.instances).toHaveLength(2)

      // Second failure backs off to 4s, but coming back online reconnects immediately.
      act(() => {
        MockEventSource.instances[1].readyState = MockEventSource.CLOSED
        MockEventSource.instances[1].onerror?.(new Event("error"))
      })
      act(() => {
        window.dispatchEvent(new Event("online"))
      })
      expect(MockEventSource.instances).toHaveLength(3)

      // Disconnecting cancels any scheduled reconnect.
      act(() => {
        MockEventSource.instances[2].readyState = MockEventSource.CLOSED
        MockEventSource.instances[2].onerror?.(new Event("error"))
        result.current.disconnectStream()
        vi.advanceTimersByTime(60_000)
      })
      expect(MockEventSource.instances).toHaveLength(3)
      expect(result.current.streamState).toBe("idle")
    } finally {
      vi.useRealTimers()
    }
  })

  it("stops SSE reconnect after repeated failures when /api/auth/me returns 401 and aborts stale probes on disconnect", async () => {
    vi.useFakeTimers()
    try {
      class MockEventSource {
        static CLOSED = 2
        static instances: MockEventSource[] = []
        readyState = 0
        onerror: ((event: Event) => void) | null = null
        onopen: ((event: Event) => void) | null = null
        constructor() {
          MockEventSource.instances.push(this)
        }
        addEventListener = vi.fn()
        close = vi.fn()
      }
      vi.stubGlobal("EventSource", MockEventSource)
      const fetchMock = vi.fn<typeof fetch>()
      vi.stubGlobal("fetch", fetchMock)
      const { result } = renderHook(() => useDashboard(), {
        wrapper: DashboardProvider,
      })

      act(() => result.current.connectStream())
      // Attempt 0 (2s) -> Attempt 1 (4s) -> Attempt 2 (8s, triggers /api/auth/me probe)
      act(() => {
        MockEventSource.instances[0].readyState = MockEventSource.CLOSED
        MockEventSource.instances[0].onerror?.(new Event("error"))
        vi.advanceTimersByTime(2_000)
      })
      act(() => {
        MockEventSource.instances[1].readyState = MockEventSource.CLOSED
        MockEventSource.instances[1].onerror?.(new Event("error"))
        vi.advanceTimersByTime(4_000)
      })

      fetchMock.mockResolvedValueOnce(
        new Response(JSON.stringify({ code: "UNAUTHORIZED" }), { status: 401 })
      )
      act(() => {
        MockEventSource.instances[2].readyState = MockEventSource.CLOSED
        MockEventSource.instances[2].onerror?.(new Event("error"))
      })
      await act(async () => {
        vi.advanceTimersByTime(8_000)
        await Promise.resolve()
        await Promise.resolve()
      })

      expect(fetchMock).toHaveBeenCalledWith(
        "/api/auth/me",
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      )
      expect(result.current.streamState).toBe("idle")
      expect(MockEventSource.instances).toHaveLength(3)

      // Disconnecting while a probe is in-flight aborts it so a stale response cannot affect a later session.
      let capturedSignal: AbortSignal | undefined
      fetchMock.mockImplementationOnce((_input, init) => {
        capturedSignal = init?.signal ?? undefined
        return new Promise<Response>(() => {})
      })
      act(() => result.current.connectStream())
      act(() => {
        MockEventSource.instances[3].readyState = MockEventSource.CLOSED
        MockEventSource.instances[3].onerror?.(new Event("error"))
        vi.advanceTimersByTime(2_000)
      })
      act(() => {
        MockEventSource.instances[4].readyState = MockEventSource.CLOSED
        MockEventSource.instances[4].onerror?.(new Event("error"))
        vi.advanceTimersByTime(4_000)
      })
      act(() => {
        MockEventSource.instances[5].readyState = MockEventSource.CLOSED
        MockEventSource.instances[5].onerror?.(new Event("error"))
        vi.advanceTimersByTime(8_000)
      })
      expect(capturedSignal?.aborted).toBe(false)

      act(() => result.current.disconnectStream())
      expect(capturedSignal?.aborted).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})
