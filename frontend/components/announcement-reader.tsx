"use client"
import { useCallback, useEffect, useRef, useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { WorkbenchButton } from "@/components/workbench/surfaces"
import { useOnline } from "@/lib/browser-state"
import {
  announcementRequest,
  announcementDate,
  type Announcement,
  type AnnouncementPage,
} from "@/lib/announcements"

export function AnnouncementReader({
  selection,
  onChange,
  onAcknowledged,
}: {
  selection: string
  onChange: (id: string | null) => void
  onAcknowledged: () => void
}) {
  const [opener] = useState(() => document.activeElement)
  const online = useOnline(),
    [page, setPage] = useState(1)
  const [data, setData] = useState<Announcement | null>(null),
    [list, setList] = useState<AnnouncementPage | null>(null)
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [changed, setChanged] = useState(false)
  const version = useRef(0),
    lock = useRef(false)
  const load = useCallback(
    async (signal?: AbortSignal) => {
      const current = version.current
      setLoading(true)
      setError("")
      setChanged(false)
      try {
        if (selection === "all") {
          const next = await announcementRequest<AnnouncementPage>(
            `/api/announcements?page=${page}`,
            { signal }
          )
          if (!signal?.aborted && current === version.current) setList(next)
        } else {
          const next = await announcementRequest<Announcement>(
            `/api/announcements/${encodeURIComponent(selection)}`,
            { signal }
          )
          if (!signal?.aborted && current === version.current) setData(next)
        }
      } catch (e) {
        if (!signal?.aborted && current === version.current) {
          setData(null)
          setList(null)
          setError(e instanceof Error ? e.message : "公告无法加载")
        }
      } finally {
        if (!signal?.aborted && current === version.current) setLoading(false)
      }
    },
    [selection, page]
  )
  useEffect(() => {
    const c = new AbortController()
    version.current++
    void Promise.resolve().then(() => {
      if (c.signal.aborted) return
      setData(null)
      setList(null)
      void load(c.signal)
    })
    return () => {
      // Invalidate pending confirmation results on navigation.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      version.current++
      c.abort()
    }
  }, [load])
  // Detect changes without silently replacing text the user is reading.
  useEffect(() => {
    if (!data) return
    const c = new AbortController()
    const check = async () => {
      if (!navigator.onLine || document.visibilityState !== "visible") return
      try {
        const next = await announcementRequest<Announcement>(
          `/api/announcements/${encodeURIComponent(data.id)}`,
          { signal: c.signal }
        )
        if (
          !c.signal.aborted &&
          (next.version !== data.version || next.status !== data.status)
        )
          setChanged(true)
      } catch {
        if (!c.signal.aborted) setChanged(true)
      }
    }
    const timer = setInterval(() => void check(), 60000)
    document.addEventListener("visibilitychange", check)
    return () => {
      c.abort()
      clearInterval(timer)
      document.removeEventListener("visibilitychange", check)
    }
  }, [data])
  async function acknowledge() {
    if (!data || lock.current || !online) return
    lock.current = true
    setBusy(true)
    setError("")
    const current = version.current
    try {
      await announcementRequest(`/api/announcements/${data.id}/acknowledge`, {
        method: "POST",
        body: JSON.stringify({
          version: data.version,
          reminderVersion: data.reminderVersion,
        }),
      })
      if (current === version.current) {
        setData({ ...data, acknowledged: true })
        onAcknowledged()
      }
    } catch (e) {
      if (current === version.current) {
        setError(e instanceof Error ? e.message : "确认失败，请重试")
        setChanged(true)
      }
    } finally {
      lock.current = false
      setBusy(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onChange(null)
      }}
    >
      <DialogContent
        finalFocus={() =>
          opener instanceof HTMLElement && opener.isConnected
            ? opener
            : document.querySelector<HTMLButtonElement>(
                "[data-announcement-list-trigger]"
              )
        }
        className="workbench-dialog max-h-[85dvh] overflow-y-auto md:!top-0 md:!right-0 md:!left-auto md:!h-dvh md:!max-h-dvh md:!w-[32rem] md:!translate-x-0 md:!translate-y-0 md:content-start md:!rounded-none"
      >
        <DialogHeader>
          <DialogTitle>
            {selection === "all" ? "全部公告" : "公告详情"}
          </DialogTitle>
          <DialogDescription>查看平台消息与有效时间。</DialogDescription>
        </DialogHeader>
        {loading ? (
          <p role="status">正在读取公告…</p>
        ) : (
          <>
            {!online && (
              <p role="status">离线，公告可能已更新。请联网后确认。</p>
            )}
            {error && <p role="alert">{error}</p>}
            {(error || changed) && (
              <div>
                <p>公告可能已更新，请重新查看。</p>
                <WorkbenchButton
                  disabled={!online || busy}
                  onClick={() => void load()}
                >
                  重新加载
                </WorkbenchButton>
              </div>
            )}
            {selection === "all" ? (
              <>
                {list?.items.map((item) => (
                  <button
                    key={item.id}
                    className="block w-full border-b border-border py-4 text-left"
                    onClick={() => onChange(item.id)}
                  >
                    <strong>{item.title}</strong>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.level === "important" ? "重要公告 · " : ""}
                      {item.status === "ended"
                        ? "已结束"
                        : item.acknowledged
                          ? "已确认"
                          : "待确认"}{" "}
                      · {announcementDate(item.startAt)}
                    </p>
                  </button>
                ))}
                {list?.items.length === 0 && <p>暂无公告</p>}
                <div className="flex items-center justify-between gap-2">
                  <WorkbenchButton
                    disabled={page === 1}
                    onClick={() => setPage(page - 1)}
                  >
                    上一页
                  </WorkbenchButton>
                  <span>第 {page} 页</span>
                  <WorkbenchButton
                    disabled={!list || page * 20 >= list.total}
                    onClick={() => setPage(page + 1)}
                  >
                    下一页
                  </WorkbenchButton>
                </div>
              </>
            ) : (
              data && (
                <article>
                  <h2 className="text-lg font-semibold">{data.title}</h2>
                  <p className="my-2 text-xs text-muted-foreground">
                    {announcementDate(data.startAt)} —{" "}
                    {data.endAt ? announcementDate(data.endAt) : "长期有效"}
                    （北京时间）{data.status === "ended" && " · 已结束"}
                  </p>
                  <p className="py-4 leading-relaxed break-words whitespace-pre-wrap">
                    {data.body}
                  </p>
                  {data.linkUrl && (
                    <a
                      className="text-primary underline"
                      href={data.linkUrl}
                      target={
                        data.linkUrl.startsWith("https:") ? "_blank" : undefined
                      }
                      rel="noopener noreferrer"
                    >
                      {data.linkLabel}
                    </a>
                  )}
                  <div className="mt-4 flex gap-2">
                    <WorkbenchButton onClick={() => onChange("all")}>
                      返回列表
                    </WorkbenchButton>
                    <WorkbenchButton
                      disabled={
                        !online ||
                        busy ||
                        changed ||
                        data.acknowledged ||
                        data.status !== "active"
                      }
                      onClick={() => void acknowledge()}
                    >
                      {busy
                        ? "正在确认…"
                        : data.acknowledged
                          ? "已确认"
                          : "我知道了"}
                    </WorkbenchButton>
                  </div>
                </article>
              )
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
