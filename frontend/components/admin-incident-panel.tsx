"use client"

import {
  CheckIcon,
  CircleAlertIcon,
  EyeIcon,
  FilterIcon,
  MessageSquareTextIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { useOnline } from "@/lib/browser-state"
import { WorkbenchSearch } from "@/components/workbench/surfaces"
import { notify } from "@/lib/feedback"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { adminApi } from "@/lib/admin-api"
import type { SystemException } from "@/lib/types"

const typeLabels: Record<string, string> = {
  credential: "凭据",
  cookie_expired: "凭据失效",
  refresh: "刷新失败",
  stale: "数据滞后",
  offline: "离线端口",
  operation: "用户操作",
  backup: "数据库备份",
  database: "数据库",
  reminder: "提醒调度",
  notification_delivery: "消息投递",
  notification_queue: "投递队列",
}

const statusLabels: Record<SystemException["status"], string> = {
  open: "未处理",
  acknowledged: "已确认",
  resolved: "已解决",
}

const filters = [
  { id: "all", label: "全部" },
  { id: "critical", label: "紧急问题" },
  { id: "credential", label: "凭据失效" },
  { id: "refresh", label: "刷新失败" },
  { id: "offline", label: "离线端口" },
  { id: "stale", label: "长期未更新" },
  { id: "notification_delivery", label: "消息投递" },
] as const

function matchesFilter(issue: SystemException, filter: string) {
  if (filter === "all") return true
  if (filter === "critical") return issue.level === "critical"
  if (filter === "credential") {
    return issue.type === "credential" || issue.type === "cookie_expired"
  }
  if (filter === "notification_delivery") {
    return (
      issue.type === "notification_delivery" ||
      issue.type === "notification_queue"
    )
  }
  return issue.type === filter
}

type AdminIncidentPanelProps = {
  initialIssues?: SystemException[]
  onUser: (id: string) => void
  compact?: boolean
}

export function AdminIncidentPanel({
  initialIssues = [],
  onUser,
  compact = false,
}: AdminIncidentPanelProps) {
  const [issues, setIssues] = useState(initialIssues)
  const [loading, setLoading] = useState(!initialIssues.length)
  const [filter, setFilter] = useState("all")
  const [status, setStatus] = useState<"active" | SystemException["status"]>(
    "active"
  )
  const [target, setTarget] = useState<SystemException | null>(null)
  const [nextStatus, setNextStatus] =
    useState<SystemException["status"]>("acknowledged")
  const [note, setNote] = useState("")
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState("")
  const [search, setSearch] = useState("")
  const online = useOnline()

  useEffect(() => {
    let ignore = false
    void adminApi
      .incidents()
      .then((next) => {
        if (!ignore) {
          setIssues(next)
          setLoadError("")
        }
      })
      .catch((reason) => {
        if (!ignore) {
          setLoadError(
            reason instanceof Error ? reason.message : "异常列表暂时不可用。"
          )
          notify.error(reason, {
            title: "异常列表加载失败",
            id: "admin-incidents-load",
          })
        }
      })
      .finally(() => {
        if (!ignore) setLoading(false)
      })
    return () => {
      ignore = true
    }
  }, [])

  const visible = useMemo(
    () =>
      issues.filter(
        (issue) =>
          matchesFilter(issue, filter) &&
          `${issue.username} ${issue.message} ${issue.deviceId ?? ""}`
            .toLocaleLowerCase()
            .includes(search.trim().toLocaleLowerCase()) &&
          (status === "active"
            ? issue.status !== "resolved"
            : issue.status === status)
      ),
    [filter, issues, status, search]
  )

  function startUpdate(
    issue: SystemException,
    desired: SystemException["status"]
  ) {
    setTarget(issue)
    setNextStatus(desired)
    setNote(issue.note ?? "")
  }

  async function save() {
    if (!target || !online) return
    setSaving(true)
    try {
      const updated = await adminApi.updateIncident(target.id, {
        status: nextStatus,
        note,
      })
      setIssues((current) =>
        current.map((issue) => (issue.id === updated.id ? updated : issue))
      )
      notify.success(`异常已标记为${statusLabels[updated.status]}`)
      setTarget(null)
    } catch (reason) {
      notify.error(reason, { title: "更新异常状态失败" })
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Card className="wb-incident-panel shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            {compact ? "需要关注" : "异常列表"}
            <Badge variant={visible.length ? "destructive" : "secondary"}>
              {visible.length} 条
            </Badge>
          </CardTitle>
          <CardDescription className="text-xs">
            重复异常会自动合并；用户异常可点击进入详情处理。
          </CardDescription>
          <CardAction>
            {compact ? (
              <Link href="/admin?tab=incidents" className="wb-text-link">
                全部异常
              </Link>
            ) : (
              <div className="flex items-center gap-1 text-xs text-muted-foreground">
                <FilterIcon className="size-3.5" />
                快捷筛选
              </div>
            )}
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {loadError && (
            <div className="wb-state-banner wb-banner-warning" role="alert">
              <div>
                <strong>异常列表未能更新</strong>
                <span>{loadError}</span>
              </div>
              <Button
                variant="outline"
                disabled={!online}
                onClick={() => {
                  setLoading(true)
                  void adminApi
                    .incidents()
                    .then((next) => {
                      setIssues(next)
                      setLoadError("")
                    })
                    .catch((reason) =>
                      setLoadError(
                        reason instanceof Error ? reason.message : "未能加载"
                      )
                    )
                    .finally(() => setLoading(false))
                }}
              >
                重试
              </Button>
            </div>
          )}
          {!compact && (
            <>
              <WorkbenchSearch
                label="搜索异常"
                placeholder="搜索异常、用户或设备"
                value={search}
                onChange={setSearch}
              />
              <div className="flex flex-wrap gap-1.5">
                {filters.map((item) => (
                  <Button
                    key={item.id}
                    size="xs"
                    variant={filter === item.id ? "default" : "outline"}
                    onClick={() => setFilter(item.id)}
                  >
                    {item.label}
                  </Button>
                ))}
              </div>
              <div className="flex flex-wrap gap-1.5 border-b pb-3">
                {(
                  [
                    ["active", "待处理"],
                    ["open", "未处理"],
                    ["acknowledged", "已确认"],
                    ["resolved", "已解决"],
                  ] as const
                ).map(([value, label]) => (
                  <Button
                    key={value}
                    size="xs"
                    variant={status === value ? "secondary" : "ghost"}
                    onClick={() => setStatus(value)}
                  >
                    {label}
                  </Button>
                ))}
              </div>
            </>
          )}
          <div
            key={`${filter}-${status}`}
            className="grid gap-2 motion-safe:animate-in motion-safe:duration-200 motion-safe:fade-in"
          >
            {loading ? (
              <>
                <Skeleton className="h-16 w-full" />
                <Skeleton className="h-16 w-full" />
              </>
            ) : visible.length ? (
              (compact ? visible.slice(0, 3) : visible).map((issue) => (
                <div
                  key={issue.id}
                  className="wb-incident-row flex flex-col gap-3 border-b py-4 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium">
                        {issue.username || "系统"} ·
                        {typeLabels[issue.type] ?? "系统"}
                      </p>
                      <Badge
                        variant={
                          issue.level === "critical"
                            ? "destructive"
                            : "secondary"
                        }
                        className={
                          issue.level === "critical"
                            ? undefined
                            : "bg-warning/15 text-warning-foreground"
                        }
                      >
                        {issue.level === "critical" ? "紧急" : "提醒"}
                      </Badge>
                      <Badge variant="outline">
                        {statusLabels[issue.status]}
                      </Badge>
                      {issue.occurrences > 1 && (
                        <Badge variant="secondary">
                          重复 {issue.occurrences} 次
                        </Badge>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {issue.message}
                      {issue.deviceId ? ` · 设备尾号 ${issue.deviceId}` : ""}
                    </p>
                    {issue.note && (
                      <p className="mt-1 flex items-center gap-1 text-xs text-primary">
                        <MessageSquareTextIcon className="size-3" />
                        {issue.note}
                      </p>
                    )}
                  </div>
                  <div
                    className="flex shrink-0 items-center gap-1.5"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {issue.userId && (
                      <Button
                        size="xs"
                        variant="ghost"
                        onClick={() => onUser(issue.userId)}
                      >
                        <EyeIcon />
                        详情
                      </Button>
                    )}
                    {issue.status === "open" && (
                      <Button
                        size="xs"
                        variant="outline"
                        disabled={!online}
                        onClick={() => startUpdate(issue, "acknowledged")}
                      >
                        <CircleAlertIcon />
                        确认
                      </Button>
                    )}
                    {issue.status !== "resolved" && (
                      <Button
                        size="xs"
                        variant="outline"
                        disabled={!online}
                        onClick={() => startUpdate(issue, "resolved")}
                      >
                        <CheckIcon />
                        解决
                      </Button>
                    )}
                  </div>
                </div>
              ))
            ) : (
              <p className="py-5 text-center text-sm text-muted-foreground">
                {loadError
                  ? "暂时无法确认是否有待处理异常。"
                  : "当前筛选下没有异常。"}
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      <Dialog
        open={Boolean(target)}
        onOpenChange={(open) => !open && setTarget(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>将异常标记为“{statusLabels[nextStatus]}”</DialogTitle>
            <DialogDescription>
              可填写处理过程或后续建议，处理人和处理时间会自动记录。
            </DialogDescription>
          </DialogHeader>
          <textarea
            value={note}
            maxLength={500}
            rows={4}
            className="w-full resize-y rounded-lg border bg-transparent px-3 py-2 text-sm transition-colors outline-none focus:border-ring focus:ring-3 focus:ring-ring/20"
            placeholder="处理备注（可选，最多 500 字）"
            onChange={(event) => setNote(event.target.value)}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setTarget(null)}>
              取消
            </Button>
            <Button disabled={saving || !online} onClick={() => void save()}>
              {saving ? "正在保存…" : "确认更新"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
