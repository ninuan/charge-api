"use client"
import { useCallback, useEffect, useRef, useState } from "react"
import { AnnouncementRevisions } from "@/components/announcement-revisions"
import { useRouter, useSearchParams } from "next/navigation"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { WorkbenchButton } from "@/components/workbench/surfaces"
import { useOnline } from "@/lib/browser-state"
import {
  announcementRequest,
  announcementDate,
  type Announcement,
  type AnnouncementPage,
} from "@/lib/announcements"

const labels: Record<string, string> = {
  draft: "草稿",
  scheduled: "待发布",
  active: "展示中",
  ended: "已结束",
  withdrawn: "已撤下",
  published: "已发布",
}
const localTime = (iso: string) =>
  new Date(new Date(iso).getTime() + 8 * 3600000).toISOString().slice(0, 16)
function blank() {
  return {
    title: "",
    body: "",
    level: "normal",
    linkLabel: "",
    linkUrl: "",
    startAt: localTime(new Date().toISOString()),
    endAt: localTime(new Date(Date.now() + 7 * 86400000).toISOString()),
    version: 0,
  }
}
export function AdminAnnouncements() {
  const router = useRouter(),
    params = useSearchParams(),
    online = useOnline()
  const id = params.get("announcement"),
    [filter, setFilter] = useState(""),
    [query, setQuery] = useState(""),
    [page, setPage] = useState(1)
  const [list, setList] = useState<AnnouncementPage | null>(null),
    [selected, setSelected] = useState<Announcement | null>(null),
    [form, setForm] = useState(blank)
  const [error, setError] = useState(""),
    [loading, setLoading] = useState(false),
    [busy, setBusy] = useState(false),
    [preview, setPreview] = useState(false),
    [confirm, setConfirm] = useState<string | null>(null),
    [revision, setRevision] = useState(0)
  const [showRevisions, setShowRevisions] = useState(false)
  const lock = useRef(false),
    epoch = useRef(0)
  const navigate = useCallback(
    (next: string | null) => {
      const p = new URLSearchParams(params.toString())
      if (next) p.set("announcement", next)
      else p.delete("announcement")
      router.push(`/admin?${p}`)
    },
    [params, router]
  )
  useEffect(() => {
    const c = new AbortController()
    epoch.current++
    void (async () => {
      await Promise.resolve()
      if (c.signal.aborted) return
      setError("")
      setLoading(true)
      setSelected(null)
      setConfirm(null)
      try {
        if (id && id !== "new") {
          const a = await announcementRequest<Announcement>(
            `/api/admin/announcements/${encodeURIComponent(id)}`,
            { signal: c.signal }
          )
          if (c.signal.aborted) return
          setSelected(a)
          setForm({
            title: a.title,
            body: a.body || "",
            level: a.level,
            linkLabel: a.linkLabel || "",
            linkUrl: a.linkUrl || "",
            startAt: localTime(a.startAt),
            endAt: a.endAt ? localTime(a.endAt) : "",
            version: a.version,
          })
        } else if (id === "new") setForm(blank())
        else {
          const data = await announcementRequest<AnnouncementPage>(
            `/api/admin/announcements?${new URLSearchParams({ status: filter, q: query, page: String(page) })}`,
            { signal: c.signal }
          )
          if (!c.signal.aborted) setList(data)
        }
      } catch (e) {
        if (!c.signal.aborted)
          setError(e instanceof Error ? e.message : "读取失败")
      } finally {
        if (!c.signal.aborted) setLoading(false)
      }
    })()
    return () => {
      // Invalidate saves after route changes.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      epoch.current++
      c.abort()
    }
  }, [id, filter, query, page, revision])
  async function mutate(action: string, again = false) {
    if (lock.current || !online) return
    lock.current = true
    setBusy(true)
    setError("")
    const current = epoch.current
    try {
      const base = "/api/admin/announcements"
      const path = id && id !== "new" ? `${base}/${id}` : base
      const payload =
        action === "save"
          ? {
              ...form,
              startAt: new Date(`${form.startAt}:00+08:00`).toISOString(),
              endAt: form.endAt
                ? new Date(`${form.endAt}:00+08:00`).toISOString()
                : null,
              remindAgain: again,
            }
          : { version: form.version }
      const result = await announcementRequest<Announcement>(
        action === "publish" || action === "withdraw"
          ? `${path}/${action}`
          : path,
        {
          method:
            action === "save"
              ? id === "new"
                ? "POST"
                : "PATCH"
              : action === "delete"
                ? "DELETE"
                : "POST",
          body: JSON.stringify(payload),
        }
      )
      if (current !== epoch.current) return
      setConfirm(null)
      if (action === "delete") navigate(null)
      else {
        if (id === "new") navigate(result.id)
        setRevision((n) => n + 1)
      }
    } catch (e) {
      if (current === epoch.current)
        setError(e instanceof Error ? e.message : "保存失败，请检查时间与内容")
    } finally {
      lock.current = false
      setBusy(false)
    }
  }
  const readOnly =
    selected?.status === "ended" || selected?.status === "withdrawn"
  return (
    <section aria-label="公告管理" className="wb-continuous">
      {error && (
        <div role="alert" className="py-3">
          {error}
          <WorkbenchButton
            disabled={busy}
            onClick={() => setRevision((n) => n + 1)}
          >
            重新加载
          </WorkbenchButton>
        </div>
      )}
      {!online && <p role="status">当前离线，联网后可保存。</p>}
      {id ? (
        <>
          <div className="flex items-center justify-between gap-3 py-3">
            <WorkbenchButton disabled={busy} onClick={() => navigate(null)}>
              返回公告列表
            </WorkbenchButton>
            <span>{selected ? labels[selected.status] : "新建草稿"}</span>
          </div>
          {loading ? (
            <p role="status">正在读取公告…</p>
          ) : (
            (id === "new" || selected) && (
              <form
                className="grid max-w-2xl gap-4 py-4"
                onSubmit={(e) => {
                  e.preventDefault()
                  void mutate("save")
                }}
              >
                <fieldset disabled={busy || readOnly} className="grid gap-4">
                  <label>
                    标题
                    <Input
                      required
                      maxLength={60}
                      value={form.title}
                      onChange={(e) =>
                        setForm({ ...form, title: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    正文
                    <textarea
                      required
                      maxLength={4000}
                      rows={8}
                      className="wb-input w-full resize-y"
                      value={form.body}
                      onChange={(e) =>
                        setForm({ ...form, body: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    提醒级别
                    <select
                      className="wb-input w-full"
                      value={form.level}
                      onChange={(e) =>
                        setForm({ ...form, level: e.target.value })
                      }
                    >
                      <option value="normal">普通 · 确认后收起</option>
                      <option value="important">重要 · 有效期内保留摘要</option>
                    </select>
                  </label>
                  <label>
                    开始时间（北京时间）
                    <Input
                      required
                      type="datetime-local"
                      value={form.startAt}
                      onChange={(e) =>
                        setForm({ ...form, startAt: e.target.value })
                      }
                    />
                  </label>
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={!form.endAt}
                      onChange={(e) =>
                        setForm({
                          ...form,
                          endAt: e.target.checked ? "" : blank().endAt,
                        })
                      }
                    />
                    长期有效
                  </label>
                  {form.endAt && (
                    <label>
                      结束时间（北京时间）
                      <Input
                        required
                        type="datetime-local"
                        value={form.endAt}
                        onChange={(e) =>
                          setForm({ ...form, endAt: e.target.value })
                        }
                      />
                    </label>
                  )}
                  <label>
                    链接文案（可选）
                    <Input
                      maxLength={20}
                      value={form.linkLabel}
                      onChange={(e) =>
                        setForm({ ...form, linkLabel: e.target.value })
                      }
                    />
                  </label>
                  <label>
                    链接地址（站内路径或 HTTPS）
                    <Input
                      maxLength={2048}
                      value={form.linkUrl}
                      onChange={(e) =>
                        setForm({ ...form, linkUrl: e.target.value })
                      }
                    />
                  </label>
                </fieldset>
                <div className="flex flex-wrap gap-2">
                  <WorkbenchButton
                    type="button"
                    onClick={() => setPreview(true)}
                  >
                    预览
                  </WorkbenchButton>
                  {!readOnly && (
                    <WorkbenchButton type="submit" disabled={busy || !online}>
                      {busy
                        ? "正在保存…"
                        : selected?.status === "draft" || !selected
                          ? "保存草稿"
                          : "保存修正（保留确认状态）"}
                    </WorkbenchButton>
                  )}
                  {selected?.status === "draft" && (
                    <>
                      <WorkbenchButton
                        type="button"
                        disabled={busy || !online}
                        onClick={() => setConfirm("publish")}
                      >
                        发布已保存草稿
                      </WorkbenchButton>
                      <WorkbenchButton
                        type="button"
                        disabled={busy || !online}
                        onClick={() => setConfirm("delete")}
                      >
                        删除草稿
                      </WorkbenchButton>
                    </>
                  )}
                  {(selected?.status === "active" ||
                    selected?.status === "scheduled") && (
                    <>
                      <WorkbenchButton
                        type="button"
                        disabled={busy || !online}
                        onClick={() => setConfirm("remind")}
                      >
                        更新并重新提醒
                      </WorkbenchButton>
                      <WorkbenchButton
                        type="button"
                        disabled={busy || !online}
                        onClick={() => setConfirm("withdraw")}
                      >
                        撤下
                      </WorkbenchButton>
                    </>
                  )}
                  {readOnly && (
                    <WorkbenchButton
                      type="button"
                      disabled={busy || !online}
                      onClick={async () => {
                        if (lock.current) return
                        lock.current = true
                        setBusy(true)
                        setError("")
                        const current = epoch.current
                        try {
                          const times = blank()
                          const result =
                            await announcementRequest<Announcement>(
                              "/api/admin/announcements",
                              {
                                method: "POST",
                                body: JSON.stringify({
                                  ...form,
                                  version: 0,
                                  startAt: new Date(
                                    `${times.startAt}:00+08:00`
                                  ).toISOString(),
                                  endAt: new Date(
                                    `${times.endAt}:00+08:00`
                                  ).toISOString(),
                                }),
                              }
                            )
                          if (current === epoch.current) navigate(result.id)
                        } catch (e) {
                          if (current === epoch.current)
                            setError(
                              e instanceof Error ? e.message : "复制失败"
                            )
                        } finally {
                          lock.current = false
                          setBusy(false)
                        }
                      }}
                    >
                      复制为新草稿
                    </WorkbenchButton>
                  )}
                </div>
                {selected && (
                  <details
                    onToggle={(event) =>
                      setShowRevisions(event.currentTarget.open)
                    }
                  >
                    <summary>查看修订记录</summary>
                    {showRevisions && (
                      <AnnouncementRevisions
                        key={`${selected.id}-${selected.version}`}
                        id={selected.id}
                      />
                    )}
                  </details>
                )}
              </form>
            )
          )}
        </>
      ) : (
        <>
          <div className="flex flex-wrap gap-3 py-4">
            <WorkbenchButton onClick={() => navigate("new")}>
              新建公告
            </WorkbenchButton>
            <Input
              aria-label="搜索公告标题"
              placeholder="搜索标题"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setPage(1)
              }}
            />
            <select
              aria-label="公告状态"
              className="wb-input"
              value={filter}
              onChange={(e) => {
                setFilter(e.target.value)
                setPage(1)
              }}
            >
              <option value="">全部状态</option>
              {Object.entries(labels)
                .filter(([k]) => k !== "published")
                .map(([k, v]) => (
                  <option key={k} value={k}>
                    {v}
                  </option>
                ))}
            </select>
          </div>
          {loading ? (
            <p role="status">正在读取公告…</p>
          ) : (
            list?.items.map((a) => (
              <button
                key={a.id}
                className="block w-full border-b border-border py-4 text-left"
                onClick={() => navigate(a.id)}
              >
                <strong>{a.title}</strong>
                <p className="mt-1 text-sm text-muted-foreground">
                  {labels[a.status]} ·{" "}
                  {a.level === "important" ? "重要" : "普通"} ·{" "}
                  {announcementDate(a.startAt)}
                </p>
              </button>
            ))
          )}
          {!loading && list?.items.length === 0 && (
            <p className="py-6">没有符合条件的公告</p>
          )}
          <div className="flex items-center gap-3 py-4">
            <WorkbenchButton
              disabled={page === 1 || loading}
              onClick={() => setPage(page - 1)}
            >
              上一页
            </WorkbenchButton>
            <span>第 {page} 页</span>
            <WorkbenchButton
              disabled={!list || page * 20 >= list.total || loading}
              onClick={() => setPage(page + 1)}
            >
              下一页
            </WorkbenchButton>
          </div>
        </>
      )}
      <Dialog open={preview} onOpenChange={setPreview}>
        <DialogContent className="workbench-dialog max-h-[85dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{form.title || "公告预览"}</DialogTitle>
            <DialogDescription>
              {form.level === "important"
                ? "用户确认后，有效期内仍显示摘要。"
                : "用户点击我知道了后，主页摘要收起。"}
            </DialogDescription>
          </DialogHeader>
          <p className="break-words whitespace-pre-wrap">{form.body}</p>
          <p>
            {form.startAt.replace("T", " ")} —{" "}
            {form.endAt ? form.endAt.replace("T", " ") : "长期有效"}（北京时间）
          </p>
          {form.linkLabel && (
            <span className="text-primary">{form.linkLabel}</span>
          )}
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!confirm}
        onOpenChange={(open) => {
          if (!open && !busy) setConfirm(null)
        }}
      >
        <DialogContent className="workbench-dialog">
          <DialogHeader>
            <DialogTitle>
              {confirm === "withdraw"
                ? "撤下这条公告？"
                : confirm === "delete"
                  ? "删除未发布草稿？"
                  : confirm === "remind"
                    ? "更新并重新提醒？"
                    : "发布已保存的公告？"}
            </DialogTitle>
            <DialogDescription>
              {confirm === "withdraw"
                ? "撤下后用户将无法继续查看该公告。"
                : confirm === "remind"
                  ? "本次修改会重新提醒所有普通用户，包括已确认的用户。"
                  : confirm === "delete"
                    ? "删除后无法恢复，请确认草稿不再需要。"
                    : "发布的是最近保存的内容，未保存的编辑不会包含在内。"}
            </DialogDescription>
          </DialogHeader>
          <WorkbenchButton
            disabled={busy || !online}
            onClick={() =>
              void mutate(
                confirm === "remind" ? "save" : confirm || "",
                confirm === "remind"
              )
            }
          >
            {busy ? "正在处理…" : "确认"}
          </WorkbenchButton>
        </DialogContent>
      </Dialog>
    </section>
  )
}
