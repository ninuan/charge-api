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
import type { DashboardSnapshot, Pile } from "@/lib/types"

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
  addPile: (payload: PilePayload) => Promise<Pile>
  deletePile: (id: string) => Promise<void>
  updatePile: (
    id: string,
    payload: { name: string; address: string; sortOrder: number }
  ) => Promise<void>
  reorderPiles: (ids: string[]) => Promise<void>
  refreshFromCapture: () => Promise<DashboardSnapshot>
  updateCookie: (cookie: string) => Promise<void>
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
    const stamp = Date.parse(next.updatedAt)
    if (Number.isFinite(stamp)) {
      // 慢的 REST 响应可能晚于更新的 SSE 帧到达，旧快照直接丢弃，避免数据闪回。
      if (stamp < lastServerStampRef.current) return
      lastServerStampRef.current = stamp
    }
    setSnapshot(next)
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
      const pile = await request<Pile>(
        "/api/piles",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
          timeoutMs: remoteOperationTimeoutMs,
        },
        "添加充电桩失败，请检查桩号后重试。"
      )
      await syncAfterMutation()
      return pile
    },
    [syncAfterMutation]
  )

  const deletePile = useCallback(
    async (id: string) => {
      await request<void>(
        `/api/piles/${id}`,
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
        `/api/piles/${id}`,
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

  const updateCookie = useCallback(
    async (cookie: string) => {
      applySnapshot(
        await request<DashboardSnapshot>(
          "/api/session/cookie",
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ cookie }),
            timeoutMs: remoteOperationTimeoutMs,
          },
          "登录信息更新失败，请检查内容后重试。"
        )
      )
    },
    [applySnapshot]
  )

  const openStream = useCallback(() => {
    if (streamRef.current || typeof EventSource === "undefined") return
    setStreamState("connecting")
    const stream = new EventSource("/api/stream", { withCredentials: true })
    streamRef.current = stream
    stream.onopen = () => {
      setStreamState("connected")
      streamOpenListenersRef.current.forEach((listener) => listener())
    }
    stream.addEventListener("snapshot", (event) => {
      try {
        applySnapshot(
          JSON.parse((event as MessageEvent<string>).data) as DashboardSnapshot
        )
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
        setStreamState("connected")
      } catch {
        setStreamState("error")
      }
    })
    stream.onerror = () => setStreamState("error")
  }, [applySnapshot])

  const connectStream = useCallback(() => {
    streamWantedRef.current = true
    // 1.5.1 的浏览器提醒依赖网页打开期间的 SSE，后台标签页也保持连接。
    openStream()
  }, [openStream])

  const disconnectStream = useCallback(() => {
    streamWantedRef.current = false
    streamRef.current?.close()
    streamRef.current = null
    setStreamState("idle")
  }, [])

  useEffect(
    () => () => {
      streamRef.current?.close()
      streamRef.current = null
    },
    []
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
      updateCookie,
      connectStream,
      disconnectStream,
      subscribeNotifications,
      subscribeStreamOpen,
    }),
    [
      addPile,
      applySnapshot,
      connectStream,
      deletePile,
      disconnectStream,
      fetchSnapshot,
      loading,
      reorderPiles,
      refreshFromCapture,
      reset,
      snapshot,
      subscribeNotifications,
      subscribeStreamOpen,
      streamState,
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
