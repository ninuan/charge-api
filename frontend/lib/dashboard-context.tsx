"use client"

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"

import { request } from "@/lib/http"
import type { Notification } from "@/lib/api/generated"
import type {
  DashboardSnapshot,
  Pile,
  PileCreateResponse,
  SharedPile,
} from "@/lib/types"

const initialSnapshot: DashboardSnapshot = {
  piles: [],
  updatedAt: new Date().toISOString(),
  statistics: {
    pileCount: 0,
    portCount: 0,
    inUsePortCount: 0,
    idlePortCount: 0,
    offlinePorts: 0,
  },
  refresh: {
    minIntervalSeconds: 30,
    attemptedDevices: 0,
    successfulDevices: 0,
    failedDevices: 0,
    skippedDevices: 0,
    cached: false,
    partial: false,
  },
}

// 刷新和添加桩会串联多次上游 12 秒超时的远端请求，比默认超时放宽。
const remoteOperationTimeoutMs = 120_000
const sseReconnectBaseMs = 2_000
const sseReconnectMaxMs = 30_000

type PilePayload = Pick<
  Pile,
  "id" | "name" | "number" | "openNum" | "status" | "address"
>

type DashboardContextValue = {
  snapshot: DashboardSnapshot
  loading: boolean
  streamState: "idle" | "connecting" | "connected" | "error"
  setSnapshot: (snapshot: DashboardSnapshot) => void
  reset: () => void
  fetchSnapshot: () => Promise<void>
  addPile: (payload: PilePayload) => Promise<PileCreateResponse>
  deletePile: (id: string) => Promise<void>
  updatePile: (
    id: string,
    payload: { name: string; address: string; sortOrder: number }
  ) => Promise<void>
  reorderPiles: (ids: string[]) => Promise<void>
  refreshFromCapture: () => Promise<DashboardSnapshot>
  refreshPile: (id: string) => Promise<DashboardSnapshot>
  searchSharedPiles: (query: string) => Promise<SharedPile[]>
  updateCookie: (cookie: string, joinPublicPool?: boolean) => Promise<void>
  testCookie: () => Promise<{ valid: boolean; error?: string }>
  joinPublicPool: () => Promise<void>
  leavePublicPool: () => Promise<void>
  clearCookie: () => Promise<void>
  connectStream: () => void
  disconnectStream: () => void
  subscribeNotifications: (
    listener: (notification: Notification) => void
  ) => () => void
  subscribeStreamOpen: (listener: () => void) => () => void
}

const DashboardContext = createContext<DashboardContextValue | null>(null)

export function DashboardProvider({ children }: { children: ReactNode }) {
  const [snapshot, setSnapshot] = useState<DashboardSnapshot>(initialSnapshot)
  const [loading, setLoading] = useState(false)
  const [streamState, setStreamState] =
    useState<DashboardContextValue["streamState"]>("idle")
  const streamRef = useRef<EventSource | null>(null)
  const streamWantedRef = useRef(false)
  const reconnectTimerRef = useRef<number | null>(null)
  const reconnectAttemptRef = useRef(0)
  const openStreamRef = useRef<() => void>(() => undefined)
  const lastServerStampRef = useRef(0)
  const notificationListenersRef = useRef(
    new Set<(notification: Notification) => void>()
  )
  const streamOpenListenersRef = useRef(new Set<() => void>())

  const subscribeNotifications = useCallback(
    (listener: (notification: Notification) => void) => {
      notificationListenersRef.current.add(listener)
      return () => notificationListenersRef.current.delete(listener)
    },
    []
  )
  const subscribeStreamOpen = useCallback((listener: () => void) => {
    streamOpenListenersRef.current.add(listener)
    return () => streamOpenListenersRef.current.delete(listener)
  }, [])

  const applySnapshot = useCallback((next: DashboardSnapshot) => {
    if (!next || !Array.isArray(next.piles)) return
    const stamp = Date.parse(next.updatedAt)
    if (Number.isFinite(stamp)) {
      if (stamp <= 0 && lastServerStampRef.current > 0) return
      // 慢的 REST 响应可能晚于更新的 SSE 帧到达，旧快照直接丢弃，避免数据闪回。
      if (stamp > 0 && stamp < lastServerStampRef.current) return
      if (stamp > 0) lastServerStampRef.current = stamp
    }
    setSnapshot((current) => ({
      ...next,
      quota: next.quota ?? current.quota,
    }))
  }, [])

  const reset = useCallback(() => {
    lastServerStampRef.current = 0
    setSnapshot({ ...initialSnapshot, updatedAt: new Date().toISOString() })
  }, [])

  const fetchSnapshot = useCallback(async () => {
    setLoading(true)
    try {
      applySnapshot(
        await request<DashboardSnapshot>(
          "/api/piles",
          {},
          "暂时无法加载充电桩信息，请稍后重试。"
        )
      )
    } finally {
      setLoading(false)
    }
  }, [applySnapshot])

  const syncAfterMutation = useCallback(async () => {
    // 服务端在每次变更后都会向 SSE 推送新快照，连接在线时不再重复全量拉取。
    if (
      typeof EventSource !== "undefined" &&
      streamRef.current?.readyState === EventSource.OPEN
    )
      return
    await fetchSnapshot()
  }, [fetchSnapshot])

  const addPile = useCallback(
    async (payload: PilePayload) => {
      const pile = await request<PileCreateResponse>(
        "/api/piles",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
          timeoutMs: remoteOperationTimeoutMs,
        },
        "添加充电桩失败，请检查桩号后重试。"
      )
      setSnapshot((current) => {
        const exists = current.piles.some((item) => item.id === pile.id)
        const nextPiles = exists
          ? current.piles.map((item) => (item.id === pile.id ? pile : item))
          : [...current.piles, pile]
        return {
          ...current,
          piles: nextPiles,
          statistics: {
            ...current.statistics,
            pileCount: nextPiles.length,
          },
          quota: pile.quota ?? current.quota,
        }
      })
      await syncAfterMutation()
      return pile
    },
    [syncAfterMutation]
  )

  const deletePile = useCallback(
    async (id: string) => {
      await request<void>(
        `/api/piles/${encodeURIComponent(id)}`,
        { method: "DELETE" },
        "删除充电桩失败，请稍后重试。"
      )
      await syncAfterMutation()
    },
    [syncAfterMutation]
  )

  const updatePile = useCallback(
    async (
      id: string,
      payload: { name: string; address: string; sortOrder: number }
    ) => {
      await request<void>(
        `/api/piles/${encodeURIComponent(id)}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
        "更新充电桩失败，请稍后重试。"
      )
      await syncAfterMutation()
    },
    [syncAfterMutation]
  )

  const reorderPiles = useCallback(
    async (ids: string[]) => {
      const next = await request<DashboardSnapshot>(
        "/api/piles/order",
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ids }),
        },
        "调整充电桩顺序失败，请稍后重试。"
      )
      applySnapshot(next)
    },
    [applySnapshot]
  )

  const refreshFromCapture = useCallback(async () => {
    const next = await request<DashboardSnapshot>(
      "/api/refresh",
      { method: "POST", timeoutMs: remoteOperationTimeoutMs },
      "暂时无法刷新设备状态，请稍后重试。"
    )
    applySnapshot(next)
    return next
  }, [applySnapshot])

  const refreshPile = useCallback(
    async (id: string) => {
      const next = await request<DashboardSnapshot>(
        `/api/piles/${encodeURIComponent(id)}/refresh`,
        { method: "POST", timeoutMs: remoteOperationTimeoutMs },
        "暂时无法刷新该充电桩，请稍后重试。"
      )
      applySnapshot(next)
      return next
    },
    [applySnapshot]
  )

  const searchSharedPiles = useCallback(async (query: string) => {
    const params = new URLSearchParams()
    if (query.trim()) params.set("q", query.trim())
    const suffix = params.toString() ? `?${params.toString()}` : ""
    return request<SharedPile[]>(
      `/api/shared-piles${suffix}`,
      {},
      "暂时无法搜索公共充电桩库，请稍后重试。"
    )
  }, [])

  const updateCookie = useCallback(
    async (cookie: string, joinPublicPool = true) => {
      applySnapshot(
        await request<DashboardSnapshot>(
          "/api/session/cookie",
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ cookie, joinPublicPool }),
            timeoutMs: remoteOperationTimeoutMs,
          },
          "登录信息更新失败，请检查内容后重试。"
        )
      )
    },
    [applySnapshot]
  )

  const joinPublicPool = useCallback(async () => {
    applySnapshot(
      await request<DashboardSnapshot>(
        "/api/session/cookie/join-pool",
        { method: "POST", timeoutMs: remoteOperationTimeoutMs },
        "加入互助公共池失败，请确认当前 Cookie 仍然有效。"
      )
    )
  }, [applySnapshot])

  const leavePublicPool = useCallback(async () => {
    applySnapshot(
      await request<DashboardSnapshot>(
        "/api/session/cookie/join-pool",
        { method: "DELETE" },
        "退出互助公共池失败，请稍后重试。"
      )
    )
  }, [applySnapshot])

  const testCookie = useCallback(async () => {
    const res = await request<{
      valid: boolean
      error?: string
      snapshot?: DashboardSnapshot
    }>(
      "/api/session/cookie/test",
      { method: "POST", timeoutMs: remoteOperationTimeoutMs },
      "测试个人 Cookie 失败，请稍后重试。"
    )
    if (res.snapshot && Array.isArray(res.snapshot.piles)) {
      applySnapshot(res.snapshot)
    }
    return { valid: res.valid, error: res.error }
  }, [applySnapshot])

  const clearCookie = useCallback(async () => {
    applySnapshot(
      await request<DashboardSnapshot>(
        "/api/session/cookie",
        { method: "DELETE" },
        "清除个人 Cookie 失败，请稍后重试。"
      )
    )
  }, [applySnapshot])

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current)
      reconnectTimerRef.current = null
    }
  }, [])

  const scheduleReconnect = useCallback(() => {
    if (!streamWantedRef.current || reconnectTimerRef.current !== null) return
    const delay = Math.min(
      sseReconnectBaseMs * 2 ** reconnectAttemptRef.current,
      sseReconnectMaxMs
    )
    reconnectAttemptRef.current += 1
    reconnectTimerRef.current = window.setTimeout(() => {
      reconnectTimerRef.current = null
      if (streamWantedRef.current) {
        openStreamRef.current()
      }
    }, delay)
  }, [])

  const openStream = useCallback(() => {
    if (streamRef.current || typeof EventSource === "undefined") return
    clearReconnectTimer()
    setStreamState("connecting")
    const stream = new EventSource("/api/stream", { withCredentials: true })
    streamRef.current = stream
    stream.onopen = () => {
      reconnectAttemptRef.current = 0
      setStreamState("connected")
      streamOpenListenersRef.current.forEach((listener) => listener())
    }
    stream.addEventListener("snapshot", (event) => {
      try {
        applySnapshot(
          JSON.parse((event as MessageEvent<string>).data) as DashboardSnapshot
        )
        reconnectAttemptRef.current = 0
        setStreamState("connected")
      } catch {
        setStreamState("error")
      }
    })
    stream.addEventListener("notification", (event) => {
      try {
        const notification = JSON.parse(
          (event as MessageEvent<string>).data
        ) as Notification
        notificationListenersRef.current.forEach((listener) =>
          listener(notification)
        )
        reconnectAttemptRef.current = 0
        setStreamState("connected")
      } catch {
        setStreamState("error")
      }
    })
    stream.onerror = () => {
      if (stream.readyState === EventSource.CLOSED) {
        stream.close()
        if (streamRef.current === stream) {
          streamRef.current = null
        }
        setStreamState("error")
        scheduleReconnect()
        return
      }
      setStreamState("error")
    }
  }, [applySnapshot, clearReconnectTimer, scheduleReconnect])

  useEffect(() => {
    openStreamRef.current = openStream
  }, [openStream])

  useEffect(() => {
    if (typeof window === "undefined") return
    const handleOnline = () => {
      if (!streamWantedRef.current || streamRef.current) return
      reconnectAttemptRef.current = 0
      clearReconnectTimer()
      openStreamRef.current()
    }
    window.addEventListener("online", handleOnline)
    return () => window.removeEventListener("online", handleOnline)
  }, [clearReconnectTimer])

  const connectStream = useCallback(() => {
    streamWantedRef.current = true
    reconnectAttemptRef.current = 0
    // 1.5.1 的浏览器提醒依赖网页打开期间的 SSE，后台标签页也保持连接。
    openStream()
  }, [openStream])

  const disconnectStream = useCallback(() => {
    streamWantedRef.current = false
    reconnectAttemptRef.current = 0
    clearReconnectTimer()
    streamRef.current?.close()
    streamRef.current = null
    setStreamState("idle")
  }, [clearReconnectTimer])

  useEffect(
    () => () => {
      streamWantedRef.current = false
      clearReconnectTimer()
      streamRef.current?.close()
      streamRef.current = null
    },
    [clearReconnectTimer]
  )

  const value = useMemo(
    () => ({
      snapshot,
      loading,
      streamState,
      setSnapshot: applySnapshot,
      reset,
      fetchSnapshot,
      addPile,
      deletePile,
      updatePile,
      reorderPiles,
      refreshFromCapture,
      refreshPile,
      searchSharedPiles,
      updateCookie,
      testCookie,
      joinPublicPool,
      leavePublicPool,
      clearCookie,
      connectStream,
      disconnectStream,
      subscribeNotifications,
      subscribeStreamOpen,
    }),
    [
      addPile,
      applySnapshot,
      clearCookie,
      connectStream,
      deletePile,
      disconnectStream,
      fetchSnapshot,
      loading,
      reorderPiles,
      refreshFromCapture,
      refreshPile,
      reset,
      searchSharedPiles,
      snapshot,
      subscribeNotifications,
      subscribeStreamOpen,
      streamState,
      joinPublicPool,
      leavePublicPool,
      testCookie,
      updateCookie,
      updatePile,
    ]
  )

  return (
    <DashboardContext.Provider value={value}>
      {children}
    </DashboardContext.Provider>
  )
}

export function useDashboard() {
  const context = useContext(DashboardContext)
  if (!context)
    throw new Error("useDashboard must be used within DashboardProvider")
  return context
}
