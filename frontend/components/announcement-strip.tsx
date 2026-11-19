"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import dynamic from "next/dynamic"
import { useSearchParams } from "next/navigation"
import { useOnline } from "@/lib/browser-state"
import { useAuth } from "@/lib/auth-context"
import { updatePageQuery } from "@/lib/workbench"
import {
  announcementRequest,
  announcementDate,
  type AnnouncementSummary,
} from "@/lib/announcements"
import { WorkbenchButton } from "@/components/workbench/surfaces"
const Details = dynamic(
  () => import("./announcement-reader").then((m) => m.AnnouncementReader),
  { ssr: false, loading: () => <p role="status">正在打开公告…</p> }
)

export function AnnouncementStrip() {
  const { currentUser } = useAuth()
  return currentUser ? <UserAnnouncements key={currentUser.id} /> : null
}
function UserAnnouncements() {
  const params = useSearchParams(),
    online = useOnline()
  const [summary, setSummary] = useState<AnnouncementSummary | null>(null)
  const [error, setError] = useState("")
  const operation = useRef<AbortController | null>(null),
    last = useRef(0),
    boundary = useRef(Number.POSITIVE_INFINITY)
  const open = params.get("announcement")
  const load = useCallback(async (force = false) => {
    if (
      !navigator.onLine ||
      (!force && operation.current) ||
      (!force &&
        Date.now() < boundary.current &&
        Date.now() - last.current < 60000)
    )
      return
    // An acknowledgement must supersede a refresh that started before it.
    operation.current?.abort()
    const controller = new AbortController()
    operation.current = controller
    last.current = Date.now()
    try {
      const data = await announcementRequest<AnnouncementSummary>(
        "/api/announcements/summary",
        { signal: controller.signal }
      )
      if (!controller.signal.aborted) {
        boundary.current = data.nextBoundary
          ? Date.now() +
            Math.max(
              0,
              Date.parse(data.nextBoundary) - Date.parse(data.serverNow)
            )
          : Number.POSITIVE_INFINITY
        setSummary(data)
        setError("")
      }
    } catch {
      if (!controller.signal.aborted) setError("公告暂时无法加载")
    } finally {
      if (operation.current === controller) operation.current = null
    }
  }, [])
  useEffect(() => {
    let cancelled = false
    void Promise.resolve().then(() => {
      if (!cancelled) void load(true)
    })
    const wake = () => {
      if (document.visibilityState === "visible") void load()
    }
    const timer = setInterval(wake, 300000)
    document.addEventListener("visibilitychange", wake)
    window.addEventListener("online", wake)
    return () => {
      cancelled = true
      clearInterval(timer)
      document.removeEventListener("visibilitychange", wake)
      window.removeEventListener("online", wake)
      operation.current?.abort()
      operation.current = null
    }
  }, [load])
  useEffect(() => {
    if (!summary?.nextBoundary) return
    const delay = Math.max(
      1000,
      new Date(summary.nextBoundary).getTime() -
        new Date(summary.serverNow).getTime() +
        100
    )
    const timer = setTimeout(
      () => {
        if (document.visibilityState === "visible") void load(true)
      },
      Math.min(delay, 2147483647)
    )
    return () => clearTimeout(timer)
  }, [summary, load])
  const item = summary?.item
  const urgent = Boolean(item?.level === "important" && !item.acknowledged && !error)
  return (
    <>
      <section
        aria-label="公告"
        className={`wb-ann-bar ${urgent ? "wb-ann-urgent" : ""}`}
      >
        <span className="wb-ann-tag">
          {item?.level === "important" ? "重要公告" : "公告"}
        </span>
        <div className="wb-ann-text">
          <p className="wb-ann-headline">
            {!online
              ? "离线，公告可能已更新"
              : error ||
                item?.title ||
                (summary ? "暂无待确认公告" : "正在读取公告…")}
          </p>
          {item && (
            <span className="wb-ann-meta">
              {item.acknowledged
                ? "已确认"
                : `${summary?.unreadCount} 条待确认`}{" "}
              ·{" "}
              <time dateTime={item.startAt}>
                {announcementDate(item.startAt)}
              </time>
            </span>
          )}
        </div>
        <div className="wb-ann-ops">
          {error ? (
            <WorkbenchButton
              size="sm"
              variant="outline"
              disabled={!online}
              onClick={() => void load(true)}
            >
              重试
            </WorkbenchButton>
          ) : (
            item && (
              <WorkbenchButton
                size="sm"
                variant={urgent ? "default" : "outline"}
                onClick={() => updatePageQuery({ announcement: item.id })}
              >
                查看
              </WorkbenchButton>
            )
          )}
          <button
            data-announcement-list-trigger
            className="wb-ann-all"
            onClick={() => updatePageQuery({ announcement: "all" })}
          >
            全部公告
          </button>
        </div>
      </section>
      {open && (
        <Details
          selection={open}
          onChange={(id) => updatePageQuery({ announcement: id })}
          onAcknowledged={() => void load(true)}
        />
      )}
    </>
  )
}
