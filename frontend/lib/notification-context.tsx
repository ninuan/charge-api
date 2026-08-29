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

import type {
  Notification as AppNotification,
  NotificationStatusFilter,
} from "@/lib/api/generated"
import { useDashboard } from "@/lib/dashboard-context"
import { navigateToNotification } from "@/lib/notification-navigation"
import {
  notificationPresentation,
  notificationRequiresAction,
} from "@/lib/notification-semantics"
import { isQuietHours } from "@/lib/notification-time"
import { watchApi } from "@/lib/watch-api"
import { useWatch } from "@/lib/watch-context"

export type BrowserPermissionState = NotificationPermission | "unsupported"

type NotificationContextValue = {
  items: AppNotification[]
  unreadCount: number
  nextCursor?: string
  status: NotificationStatusFilter
  loading: boolean
  loadingMore: boolean
  loaded: boolean
  error: string | null
  browserPermission: BrowserPermissionState
  load: (status?: NotificationStatusFilter) => Promise<void>
  loadMore: () => Promise<void>
  markRead: (notificationId: string) => Promise<void>
  markAllRead: () => Promise<number>
  clearResolved: () => Promise<number>
  requestBrowserPermission: () => Promise<BrowserPermissionState>
  setBrowserEnabled: (enabled: boolean) => Promise<boolean>
}

const NotificationContext = createContext<NotificationContextValue | null>(null)

function currentPermission(): BrowserPermissionState {
  return typeof window === "undefined" || !("Notification" in window)
    ? "unsupported"
    : window.Notification.permission
}

function mergeNotifications(
  current: AppNotification[],
  incoming: AppNotification[]
) {
  const merged = new Map(
    current.map((notification) => [notification.id, notification])
  )
  for (const notification of incoming) merged.set(notification.id, notification)
  return Array.from(merged.values())
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  const { subscribeNotifications, subscribeStreamOpen } = useDashboard()
  const { preference, updatePreference } = useWatch()
  const [items, setItems] = useState<AppNotification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [nextCursor, setNextCursor] = useState<string>()
  const [status, setStatus] = useState<NotificationStatusFilter>("all")
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [browserPermission, setBrowserPermission] =
    useState<BrowserPermissionState>("unsupported")
  const requestVersionRef = useRef(0)
  const loadedRef = useRef(false)
  const statusRef = useRef<NotificationStatusFilter>("all")
  const preferenceRef = useRef(preference)
  const lastLoadAtRef = useRef(0)
  const streamedNotificationVersionsRef = useRef(new Map<string, string>())
  const notificationUnreadStateRef = useRef(new Map<string, boolean>())

  useEffect(() => {
    preferenceRef.current = preference
  }, [preference])

  useEffect(() => {
    const refreshPermission = () => setBrowserPermission(currentPermission())
    refreshPermission()
    window.addEventListener("focus", refreshPermission)
    return () => window.removeEventListener("focus", refreshPermission)
  }, [])

  const load = useCallback(
    async (nextStatus: NotificationStatusFilter = statusRef.current) => {
      const version = ++requestVersionRef.current
      statusRef.current = nextStatus
      setStatus(nextStatus)
      setLoading(true)
      setError(null)
      try {
        const page = await watchApi.notifications({ status: nextStatus })
        if (version !== requestVersionRef.current) return
        setItems(page.items)
        notificationUnreadStateRef.current = new Map(
          page.items.map((notification) => [
            notification.id,
            !notification.readAt,
          ])
        )
        setUnreadCount(page.unreadCount)
        setNextCursor(page.nextCursor)
        setLoaded(true)
        loadedRef.current = true
        lastLoadAtRef.current = Date.now()
      } catch (reason) {
        if (version !== requestVersionRef.current) return
        setError((reason as Error).message)
        throw reason
      } finally {
        if (version === requestVersionRef.current) setLoading(false)
      }
    },
    []
  )

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return
    setLoadingMore(true)
    setError(null)
    try {
      const page = await watchApi.notifications({
        status: statusRef.current,
        cursor: nextCursor,
      })
      setItems((current) => mergeNotifications(current, page.items))
      for (const notification of page.items)
        notificationUnreadStateRef.current.set(
          notification.id,
          !notification.readAt
        )
      setUnreadCount(page.unreadCount)
      setNextCursor(page.nextCursor)
    } catch (reason) {
      setError((reason as Error).message)
      throw reason
    } finally {
      setLoadingMore(false)
    }
  }, [loadingMore, nextCursor])

  const markRead = useCallback(
    async (notificationId: string) => {
      const updated = await watchApi.markNotificationRead(notificationId)
      notificationUnreadStateRef.current.set(updated.id, false)
      setItems((current) =>
        statusRef.current === "unread"
          ? current.filter((notification) => notification.id !== updated.id)
          : current.map((notification) =>
              notification.id === updated.id ? updated : notification
            )
      )
      setUnreadCount((current) =>
        items.some(
          (notification) =>
            notification.id === notificationId && !notification.readAt
        )
          ? Math.max(0, current - 1)
          : current
      )
    },
    [items]
  )

  const markAllRead = useCallback(async () => {
    const result = await watchApi.markAllNotificationsRead()
    const readAt = new Date().toISOString()
    setItems((current) =>
      statusRef.current === "unread"
        ? []
        : current.map((notification) =>
            notification.readAt ? notification : { ...notification, readAt }
          )
    )
    setUnreadCount(0)
    for (const id of notificationUnreadStateRef.current.keys())
      notificationUnreadStateRef.current.set(id, false)
    return result.updated
  }, [])

  const clearResolved = useCallback(async () => {
    const result = await watchApi.clearResolvedNotifications()
    setItems((current) =>
      current.filter((notification) => !notification.resolvedAt)
    )
    return result.deleted
  }, [])

  const requestBrowserPermission = useCallback(async () => {
    if (!("Notification" in window)) {
      setBrowserPermission("unsupported")
      return "unsupported" as const
    }
    const permission = await window.Notification.requestPermission()
    setBrowserPermission(permission)
    if (permission === "granted") {
      await updatePreference({ browserEnabled: true })
    } else if (preferenceRef.current?.browserEnabled) {
      await updatePreference({ browserEnabled: false })
    }
    return permission
  }, [updatePreference])

  const setBrowserEnabled = useCallback(
    async (enabled: boolean) => {
      const permission = currentPermission()
      setBrowserPermission(permission)
      if (enabled && permission === "default")
        return (await requestBrowserPermission()) === "granted"
      if (enabled && permission !== "granted") return false
      await updatePreference({ browserEnabled: enabled })
      return enabled
    },
    [requestBrowserPermission, updatePreference]
  )

  useEffect(() => {
    const unsubscribeNotification = subscribeNotifications((notification) => {
      const version = `${notification.lastOccurredAt}:${notification.readAt ?? ""}:${notification.resolvedAt ?? ""}`
      if (
        streamedNotificationVersionsRef.current.get(notification.id) === version
      )
        return
      streamedNotificationVersionsRef.current.set(notification.id, version)
      const previousUnread = notificationUnreadStateRef.current.get(
        notification.id
      )
      const nextUnread = !notification.readAt
      if (nextUnread && previousUnread === false)
        setUnreadCount((count) => count + 1)
      else if (!nextUnread && previousUnread === true)
        setUnreadCount((count) => Math.max(0, count - 1))
      else if (
        nextUnread &&
        previousUnread === undefined &&
        (notification.occurrenceCount ?? 1) === 1
      )
        setUnreadCount((count) => count + 1)
      notificationUnreadStateRef.current.set(notification.id, nextUnread)
      setItems((current) => {
        const requiresAction = notificationRequiresAction(notification.type)
        const visible =
          statusRef.current === "all" ||
          (statusRef.current === "unread" && !notification.readAt) ||
          (statusRef.current === "pending" &&
            requiresAction &&
            !notification.resolvedAt) ||
          (statusRef.current === "resolved" &&
            requiresAction &&
            Boolean(notification.resolvedAt))
        const withoutCurrent = current.filter(
          (item) => item.id !== notification.id
        )
        return visible ? [notification, ...withoutCurrent] : withoutCurrent
      })

      const currentPreference = preferenceRef.current
      if (
        notification.type === "pile_recovered" ||
        !currentPreference?.browserEnabled ||
        currentPermission() !== "granted" ||
        isQuietHours(currentPreference)
      )
        return

      const presentation = notificationPresentation(notification)
      const browserNotification = new window.Notification(presentation.title, {
        body: presentation.message,
        tag: notification.id,
      })
      browserNotification.onclick = () => {
        window.focus()
        navigateToNotification(notification)
        browserNotification.close()
      }
    })
    const unsubscribeOpen = subscribeStreamOpen(() => {
      if (!loadedRef.current || Date.now() - lastLoadAtRef.current < 1_000)
        return
      void load(statusRef.current).catch(() => undefined)
    })
    return () => {
      unsubscribeNotification()
      unsubscribeOpen()
    }
  }, [load, subscribeNotifications, subscribeStreamOpen])

  const value = useMemo<NotificationContextValue>(
    () => ({
      items,
      unreadCount,
      nextCursor,
      status,
      loading,
      loadingMore,
      loaded,
      error,
      browserPermission,
      load,
      loadMore,
      markRead,
      markAllRead,
      clearResolved,
      requestBrowserPermission,
      setBrowserEnabled,
    }),
    [
      browserPermission,
      clearResolved,
      error,
      items,
      load,
      loaded,
      loading,
      loadingMore,
      loadMore,
      markAllRead,
      markRead,
      nextCursor,
      requestBrowserPermission,
      setBrowserEnabled,
      status,
      unreadCount,
    ]
  )

  return (
    <NotificationContext.Provider value={value}>
      {children}
    </NotificationContext.Provider>
  )
}

export function useNotifications() {
  const value = useContext(NotificationContext)
  if (!value)
    throw new Error("useNotifications must be used inside NotificationProvider")
  return value
}
