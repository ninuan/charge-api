"use client"
import { useEffect, useState } from "react"
import {
  announcementRequest,
  announcementDate,
  type AnnouncementPage,
} from "@/lib/announcements"
import { WorkbenchButton } from "@/components/workbench/surfaces"
export function AnnouncementRevisions({ id }: { id: string }) {
  const [page, setPage] = useState(1),
    [data, setData] = useState<AnnouncementPage | null>(null),
    [error, setError] = useState(""),
    [revision, setRevision] = useState(0)
  useEffect(() => {
    const c = new AbortController()
    void announcementRequest<AnnouncementPage>(
      `/api/admin/announcements/${id}/revisions?page=${page}`,
      { signal: c.signal }
    )
      .then((next) => {
        if (!c.signal.aborted) {
          setData(next)
          setError("")
        }
      })
      .catch(() => {
        if (!c.signal.aborted) setError("修订记录暂时无法读取")
      })
    return () => c.abort()
  }, [id, page, revision])
  return (
    <section aria-label="修订记录" className="border-t border-border py-4">
      <h3 className="font-semibold">修订记录</h3>
      {error && (
        <p role="alert">
          {error}
          <WorkbenchButton onClick={() => setRevision((n) => n + 1)}>
            重试
          </WorkbenchButton>
        </p>
      )}
      {!data && !error && <p role="status">正在读取修订…</p>}
      {data?.items.map((a) => (
        <details key={a.version} className="border-b border-border py-3">
          <summary>
            版本 {a.version} · 提醒版本 {a.reminderVersion} ·{" "}
            {announcementDate(a.updatedAt)}
          </summary>
          <h4 className="py-2 font-medium">{a.title}</h4>
          <p className="break-words whitespace-pre-wrap">{a.body}</p>
        </details>
      ))}
      <div className="flex gap-3 py-3">
        <WorkbenchButton
          disabled={page === 1}
          onClick={() => setPage(page - 1)}
        >
          较新修订
        </WorkbenchButton>
        <WorkbenchButton
          disabled={!data || page * 20 >= data.total}
          onClick={() => setPage(page + 1)}
        >
          更早修订
        </WorkbenchButton>
      </div>
    </section>
  )
}
