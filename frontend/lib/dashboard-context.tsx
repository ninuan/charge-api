"use client"

import { createContext, type ReactNode, useCallback, useContext, useMemo, useRef, useState } from "react"

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

type PilePayload = Pick<Pile, "id" | "name" | "number" | "openNum" | "status" | "address">

type DashboardContextValue = {
  snapshot: DashboardSnapshot
  loading: boolean
  streamState: "idle" | "connecting" | "connected" | "error"
  setSnapshot: (snapshot: DashboardSnapshot) => void
  reset: () => void
  fetchSnapshot: () => Promise<void>
  addPile: (payload: PilePayload) => Promise<Pile>
  deletePile: (id: string) => Promise<void>
  updatePile: (id: string, payload: { name: string; address: string; sortOrder: number }) => Promise<void>
  refreshFromCapture: () => Promise<void>
  updateCookie: (cookie: string) => Promise<void>
  connectStream: () => void
  disconnectStream: () => void
}

const DashboardContext = createContext<DashboardContextValue | null>(null)

async function throwResponseError(response: Response, fallback: string): Promise<never> {
  if (response.status === 401) {
    throw new Error("登录已失效，请重新登录")
  }

  const body = (await response.json().catch(() => ({ error: fallback }))) as { error?: string }
  throw new Error(body.error ?? fallback)
}

export function DashboardProvider({ children }: { children: ReactNode }) {
  const [snapshot, setSnapshot] = useState<DashboardSnapshot>(initialSnapshot)
  const [loading, setLoading] = useState(false)
  const [streamState, setStreamState] = useState<DashboardContextValue["streamState"]>("idle")
  const streamRef = useRef<EventSource | null>(null)

  const reset = useCallback(() => {
    setSnapshot({ ...initialSnapshot, updatedAt: new Date().toISOString() })
  }, [])

  const fetchSnapshot = useCallback(async () => {
    setLoading(true)
    try {
      const response = await fetch("/api/piles", { credentials: "include" })
      if (!response.ok) await throwResponseError(response, "Load failed")
      setSnapshot((await response.json()) as DashboardSnapshot)
    } finally {
      setLoading(false)
    }
  }, [])

  const addPile = useCallback(async (payload: PilePayload) => {
    const response = await fetch("/api/piles", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    })
    if (!response.ok) await throwResponseError(response, "add pile failed")
    const pile = (await response.json()) as Pile
    await fetchSnapshot()
    return pile
  }, [fetchSnapshot])

  const deletePile = useCallback(async (id: string) => {
    const response = await fetch(`/api/piles/${id}`, { method: "DELETE", credentials: "include" })
    if (!response.ok && response.status !== 204) await throwResponseError(response, "delete pile failed")
    await fetchSnapshot()
  }, [fetchSnapshot])

  const updatePile = useCallback(async (id: string, payload: { name: string; address: string; sortOrder: number }) => {
    const response = await fetch(`/api/piles/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    })
    if (!response.ok) await throwResponseError(response, "更新充电桩失败")
    await fetchSnapshot()
  }, [fetchSnapshot])

  const refreshFromCapture = useCallback(async () => {
    const response = await fetch("/api/refresh", { method: "POST", credentials: "include" })
    if (!response.ok) await throwResponseError(response, "refresh failed")
    setSnapshot((await response.json()) as DashboardSnapshot)
  }, [])

  const updateCookie = useCallback(async (cookie: string) => {
    const response = await fetch("/api/session/cookie", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cookie }),
    })
    if (!response.ok) await throwResponseError(response, "cookie update failed")
    setSnapshot((await response.json()) as DashboardSnapshot)
  }, [])

  const disconnectStream = useCallback(() => {
    streamRef.current?.close()
    streamRef.current = null
    setStreamState("idle")
  }, [])

  const connectStream = useCallback(() => {
    if (streamRef.current || typeof EventSource === "undefined") return
    setStreamState("connecting")
    const stream = new EventSource("/api/stream", { withCredentials: true })
    streamRef.current = stream
    stream.addEventListener("snapshot", (event) => {
      try {
        setSnapshot(JSON.parse((event as MessageEvent<string>).data) as DashboardSnapshot)
        setStreamState("connected")
      } catch {
        setStreamState("error")
      }
    })
    stream.onerror = () => setStreamState("error")
  }, [])

  const value = useMemo(() => ({
    snapshot,
    loading,
    streamState,
    setSnapshot,
    reset,
    fetchSnapshot,
    addPile,
    deletePile,
    updatePile,
    refreshFromCapture,
    updateCookie,
    connectStream,
    disconnectStream,
  }), [addPile, connectStream, deletePile, disconnectStream, fetchSnapshot, loading, refreshFromCapture, reset, snapshot, streamState, updateCookie, updatePile])

  return <DashboardContext.Provider value={value}>{children}</DashboardContext.Provider>
}

export function useDashboard() {
  const context = useContext(DashboardContext)
  if (!context) throw new Error("useDashboard must be used within DashboardProvider")
  return context
}
