"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useRouter } from "next/navigation"
import { AppShell } from "@/components/app-shell"
import { WorkbenchError, WorkbenchLoading } from "./surfaces"
import { useAuth } from "@/lib/auth-context"
import { useDashboard } from "@/lib/dashboard-context"
import { useWatch } from "@/lib/watch-context"
import { useNotifications } from "@/lib/notification-context"
import { notificationNavigateEvent } from "@/lib/notification-navigation"
import { dashboardHref } from "@/lib/workbench"
import type { Notification } from "@/lib/api/generated"

const SessionContext = createContext<{
  initialLoading: boolean
  error: string | null
  retry: () => Promise<void>
} | null>(null)

export function DashboardSession({ children }: { children: ReactNode }) {
  const router = useRouter()
  const { currentUser, fetchMe } = useAuth()
  const {
    fetchSnapshot,
    connectStream,
    disconnectStream,
    subscribeNotifications,
  } = useDashboard()
  const { load: loadWatch } = useWatch()
  const { load: loadNotifications } = useNotifications()
  const [authorized, setAuthorized] = useState(false)
  const [initialLoading, setInitialLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const version = useRef(0)
  const load = useCallback(async () => {
    const current = ++version.current
    setInitialLoading(true)
    setError(null)
    try {
      const user = currentUser ?? (await fetchMe())
      if (current !== version.current) return
      if (!user) {
        router.replace("/login")
        return
      }
      if (user.role !== "user") {
        router.replace("/admin")
        return
      }
      setAuthorized(true)
      // Auxiliary panels own their loading/error states; they must not hold
      // the pile workspace or its stream behind a slow notification request.
      void loadWatch().catch(() => undefined)
      void loadNotifications().catch(() => undefined)
      const [snapshotResult] = await Promise.allSettled([fetchSnapshot()])
      if (current !== version.current) return
      if (snapshotResult.status === "rejected")
        setError(
          snapshotResult.reason instanceof Error
            ? snapshotResult.reason.message
            : "暂时无法读取充电桩。"
        )
      connectStream()
    } catch (reason) {
      if (current === version.current)
        setError(
          reason instanceof Error ? reason.message : "暂时无法验证登录状态。"
        )
    } finally {
      if (current === version.current) setInitialLoading(false)
    }
  }, [
    currentUser,
    fetchMe,
    router,
    fetchSnapshot,
    loadWatch,
    loadNotifications,
    connectStream,
  ])
  const initialLoad = useRef(load)
  const invalidate = useCallback(() => {
    version.current += 1
  }, [])
  useEffect(() => {
    void initialLoad.current()
    return () => {
      invalidate()
      disconnectStream()
    }
  }, [disconnectStream, invalidate])
  useEffect(
    () =>
      subscribeNotifications((notification) => {
        if (notification.type === "pile_available")
          void loadWatch().catch(() => undefined)
      }),
    [loadWatch, subscribeNotifications]
  )
  useEffect(() => {
    const navigate = (event: Event) => {
      const notification = (event as CustomEvent<Notification>).detail
      router.push(
        notification.type === "credential_expired"
          ? "/account?tab=connection&connect=1"
          : dashboardHref(notification.deviceId, notification.portId)
      )
    }
    window.addEventListener(notificationNavigateEvent, navigate)
    return () => window.removeEventListener(notificationNavigateEvent, navigate)
  }, [router])
  return (
    <SessionContext.Provider value={{ initialLoading, error, retry: load }}>
      {authorized ? (
        children
      ) : (
        <AppShell title="我的充电空间" description="正在确认登录状态。">
          {error ? (
            <WorkbenchError message={error} retry={() => void load()} />
          ) : (
            <WorkbenchLoading label="正在连接账户" />
          )}
        </AppShell>
      )}
    </SessionContext.Provider>
  )
}
export function useDashboardSession() {
  const context = useContext(SessionContext)
  if (!context) throw new Error("DashboardSession is required")
  return context
}
