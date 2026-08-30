import { act, renderHook } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { NotificationPreference } from "@/lib/api/generated"
import {
  NotificationProvider,
  useNotifications,
} from "@/lib/notification-context"

const mocks = vi.hoisted(() => ({
  notificationListener: null as null | ((notification: never) => void),
  streamOpenListener: null as null | (() => void),
  updatePreference: vi.fn(),
  preference: null as NotificationPreference | null,
}))

vi.mock("@/lib/dashboard-context", () => ({
  useDashboard: () => ({
    subscribeNotifications: (listener: (notification: never) => void) => {
      mocks.notificationListener = listener
      return () => {
        mocks.notificationListener = null
      }
    },
    subscribeStreamOpen: (listener: () => void) => {
      mocks.streamOpenListener = listener
      return () => {
        mocks.streamOpenListener = null
      }
    },
  }),
}))

vi.mock("@/lib/watch-context", () => ({
  useWatch: () => ({
    preference: mocks.preference,
    updatePreference: mocks.updatePreference,
  }),
}))

const initialNotice = {
  id: "notice-1",
  userId: "user-1",
  type: "pile_available" as const,
  severity: "info" as const,
  title: "松园充电桩有空闲口",
  message: "整桩出现至少一个空闲充电口",
  deviceId: "pile-1",
  portId: 3,
  createdAt: "2026-08-11T09:00:00Z",
}

class MockBrowserNotification {
  static permission: NotificationPermission = "granted"
  static requestPermission = vi.fn(async () => "granted" as const)
  static instances: MockBrowserNotification[] = []
  onclick: (() => void) | null = null
  close = vi.fn()
  constructor(
    readonly title: string,
    readonly options?: NotificationOptions
  ) {
    MockBrowserNotification.instances.push(this)
  }
}

function wrapper({ children }: { children: ReactNode }) {
  return <NotificationProvider>{children}</NotificationProvider>
}

describe("NotificationProvider", () => {
  beforeEach(() => {
    mocks.preference = {
      userId: "user-1",
      browserEnabled: true,
      quietHoursEnabled: false,
      quietStartMinute: 1320,
      quietEndMinute: 480,
      timezone: "Asia/Shanghai",
      updatedAt: "2026-08-11T00:00:00Z",
    }
    mocks.updatePreference.mockReset()
    MockBrowserNotification.instances = []
    MockBrowserNotification.permission = "granted"
    vi.stubGlobal("Notification", MockBrowserNotification)
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ items: [initialNotice], unreadCount: 1 })
          )
        )
    )
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it("loads durable notifications and delivers a new SSE item in the browser", async () => {
    const { result } = renderHook(() => useNotifications(), { wrapper })
    await act(() => result.current.load())

    act(() => {
      mocks.notificationListener?.({
        ...initialNotice,
        id: "notice-2",
      } as never)
    })

    expect(result.current.unreadCount).toBe(2)
    expect(result.current.items[0].id).toBe("notice-2")
    expect(MockBrowserNotification.instances[0]).toMatchObject({
      title: "3 号充电口空闲了",
      options: expect.objectContaining({
        body: "整桩出现至少一个空闲充电口",
        tag: "notice-2",
      }),
    })

    act(() => {
      mocks.notificationListener?.({
        ...initialNotice,
        id: "notice-2",
      } as never)
    })
    expect(result.current.unreadCount).toBe(2)
    expect(MockBrowserNotification.instances).toHaveLength(1)
  })

  it("reloads the persisted page after a later SSE reconnect", async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date("2026-08-11T09:00:00Z"))
    const fetchMock = vi.mocked(fetch)
    const { result } = renderHook(() => useNotifications(), { wrapper })
    await act(() => result.current.load())
    vi.setSystemTime(new Date("2026-08-11T09:00:02Z"))

    act(() => mocks.streamOpenListener?.())
    await act(async () => {
      await vi.runAllTimersAsync()
    })

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("only streams actionable notifications into the pending filter", async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ items: [], unreadCount: 0 }))
    )
    const { result } = renderHook(() => useNotifications(), { wrapper })
    await act(() => result.current.load("pending"))

    act(() => {
      mocks.notificationListener?.({
        ...initialNotice,
        id: "notice-information",
      } as never)
    })
    expect(result.current.items).toHaveLength(0)

    act(() => {
      mocks.notificationListener?.({
        ...initialNotice,
        id: "notice-problem",
        type: "pile_offline",
      } as never)
    })
    expect(result.current.items.map((item) => item.id)).toEqual([
      "notice-problem",
    ])
  })

  it("reloads unread state after clearing resolved notifications", async () => {
    const resolvedUnreadNotice = {
      ...initialNotice,
      resolvedAt: "2026-08-11T09:05:00Z",
    }
    vi.mocked(fetch)
      .mockReset()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ items: [resolvedUnreadNotice], unreadCount: 1 })
        )
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ deleted: 1 })))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ items: [], unreadCount: 0 }))
      )

    const { result } = renderHook(() => useNotifications(), { wrapper })
    await act(() => result.current.load("resolved"))
    expect(result.current.unreadCount).toBe(1)

    await act(() => result.current.clearResolved())

    expect(result.current.items).toEqual([])
    expect(result.current.unreadCount).toBe(0)
    expect(fetch).toHaveBeenCalledTimes(3)
  })
})
