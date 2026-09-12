"use client"

import {
  ArrowLeftIcon,
  ArrowRightIcon,
  BellIcon,
  CheckCheckIcon,
  ChevronRightIcon,
  InboxIcon,
  ShieldAlertIcon,
  Trash2Icon,
} from "lucide-react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useEffect, useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type {
  Notification,
  NotificationStatusFilter,
} from "@/lib/api/generated"
import { useNotifications } from "@/lib/notification-context"
import {
  notificationPresentation,
  notificationRequiresAction,
} from "@/lib/notification-semantics"
import { useDashboard } from "@/lib/dashboard-context"
import { useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import {
  dashboardHref,
  formatSnapshotTime,
  updatePageQuery,
} from "@/lib/workbench"
import {
  StatusPill,
  WorkbenchButton,
  WorkbenchEmpty,
  WorkbenchError,
  WorkbenchLoading,
  WorkbenchSearch,
} from "./surfaces"
import { UserWorkbenchShell } from "./user-shell"

const filters: [NotificationStatusFilter, string][] = [
  ["all", "全部"],
  ["unread", "未读"],
  ["pending", "待处理"],
  ["resolved", "已解决"],
]
export function NotificationsPage() {
  return (
    <Suspense fallback={<WorkbenchLoading />}>
      <NotificationsContent />
    </Suspense>
  )
}
function NotificationsContent() {
  const router = useRouter(),
    params = useSearchParams(),
    online = useOnline()
  const { snapshot } = useDashboard()
  const {
    items,
    unreadCount,
    nextCursor,
    loading,
    loadingMore,
    loaded,
    error,
    load,
    loadMore,
    markRead,
    markAllRead,
    clearResolved,
  } = useNotifications()
  const status = filters.some(([value]) => value === params.get("status"))
    ? (params.get("status") as NotificationStatusFilter)
    : "all"
  const query = params.get("q") ?? "",
    selectedId = params.get("notice")
  const [opened, setOpened] = useState<Notification | null>(null)
  const [readId, setReadId] = useState<string | null>(null)
  const [confirmClear, setConfirmClear] = useState(false)
  const [pending, setPending] = useState(false)
  const selected =
    items.find((item) => item.id === selectedId) ||
    (opened?.id === selectedId ? opened : null)
  const shown = items
    .filter(
      (item) =>
        status === "all" ||
        (status === "unread"
          ? !item.readAt
          : notificationRequiresAction(item.type) &&
            (status === "pending" ? !item.resolvedAt : !!item.resolvedAt))
    )
    .filter((item) =>
      `${notificationPresentation(item).title} ${item.message}`.includes(
        query.trim()
      )
    )
  useEffect(() => {
    void load(status).catch(() => undefined)
  }, [load, status])
  async function open(item: Notification) {
    setOpened(item)
    setReadId(null)
    updatePageQuery({ notice: item.id })
    if (!item.readAt && online)
      try {
        await markRead(item.id)
        setReadId(item.id)
      } catch (reason) {
        notify.error(reason, {
          title: "暂时未能标为已读",
          description: "消息仍保留，可以继续查看内容。",
        })
      }
  }
  async function readAll() {
    setPending(true)
    try {
      const count = await markAllRead()
      notify.success(count ? `已标记 ${count} 条通知为已读` : "没有未读通知")
    } catch (reason) {
      notify.error(reason, { title: "标记通知失败" })
    } finally {
      setPending(false)
    }
  }
  async function clear() {
    setPending(true)
    try {
      const count = await clearResolved()
      setConfirmClear(false)
      notify.success(`已清理 ${count} 条已解决通知`)
    } catch (reason) {
      notify.error(reason, { title: "清理未完成" })
    } finally {
      setPending(false)
    }
  }
  function target(item: Notification) {
    router.push(
      item.type === "credential_expired"
        ? "/account?tab=connection&connect=1"
        : dashboardHref(item.deviceId, item.portId)
    )
  }
  return (
    <UserWorkbenchShell
      title="通知"
      activeSection="notifications"
      description={
        unreadCount
          ? `${unreadCount} 条未读。空闲消息和需要处理的事情，都在这里。`
          : "空闲消息与需要处理的事情，都在这里。"
      }
      actions={
        <>
          <WorkbenchButton
            variant="ghost"
            busy={pending}
            disabled={!online || !unreadCount}
            onClick={() => void readAll()}
          >
            <CheckCheckIcon size={15} />
            全部已读
          </WorkbenchButton>
          <WorkbenchButton
            variant="ghost"
            disabled={!online || pending}
            onClick={() => setConfirmClear(true)}
          >
            <Trash2Icon size={14} />
            清理已解决
          </WorkbenchButton>
        </>
      }
    >
      <div className="wb-standard-page">
        <div className="wb-page-rule">
          <div className="wb-section-tabs">
            {filters.map(([value, label]) => (
              <button
                aria-pressed={status === value}
                key={value}
                onClick={() =>
                  updatePageQuery({
                    status: value === "all" ? null : value,
                    notice: null,
                  })
                }
              >
                {label}
                {value === "unread" && !!unreadCount && (
                  <span>{unreadCount}</span>
                )}
              </button>
            ))}
          </div>
          <WorkbenchSearch
            label="搜索通知"
            placeholder="搜索已加载的通知"
            value={query}
            onChange={(value) =>
              updatePageQuery({ q: value, notice: null }, true)
            }
          />
        </div>
        {selected ? (
          <section className="wb-notice-detail">
            <button
              className="wb-back"
              onClick={() => updatePageQuery({ notice: null })}
            >
              <ArrowLeftIcon size={15} />
              返回通知列表
            </button>
            <div className="wb-notice-detail-title">
              <span className="wb-notice-icon">
                <BellIcon size={23} />
              </span>
              <div>
                <h2>{notificationPresentation(selected).title}</h2>
                <p>
                  {formatSnapshotTime(selected.createdAt, true)} ·{" "}
                  {selected.readAt || readId === selected.id ? "已读" : "未读"}
                  {notificationRequiresAction(selected.type)
                    ? selected.resolvedAt
                      ? " · 问题已解决"
                      : " · 仍需处理"
                    : ""}
                </p>
              </div>
            </div>
            <p className="wb-notice-body">
              {notificationPresentation(selected).message}
            </p>
            {selected.deviceId && (
              <p className="wb-notice-target-name">
                {snapshot.piles.find((pile) => pile.id === selected.deviceId)
                  ?.name || "关联充电桩"}
                {selected.portId ? ` · ${selected.portId} 号口` : ""}
              </p>
            )}
            <div className="wb-delivery-fact">
              <InboxIcon size={18} />
              <div>
                <strong>消息已保存在站内通知</strong>
                <p>
                  {selected.type === "pile_available"
                    ? "空闲消息记录的是发现时的状态，出发前请再确认；这次提醒已结束，不预留充电口。"
                    : "已读只代表你看过这条消息，不会自动解决连接问题。"}
                </p>
                <p>外部渠道的提交状态不等于手机送达或已读。</p>
              </div>
            </div>
            <WorkbenchButton onClick={() => target(selected)}>
              {selected.type === "credential_expired"
                ? "检查平台连接"
                : "查看充电桩"}
              <ArrowRightIcon size={15} />
            </WorkbenchButton>
          </section>
        ) : selectedId && !loading ? (
          <WorkbenchEmpty
            title="这条通知尚未加载"
            description="可以返回列表，或继续加载更早的消息。"
            action={
              <WorkbenchButton
                variant="outline"
                onClick={() =>
                  nextCursor
                    ? void loadMore().catch(() => undefined)
                    : updatePageQuery({ notice: null })
                }
              >
                {nextCursor ? "加载更早通知" : "返回通知列表"}
              </WorkbenchButton>
            }
          />
        ) : loading && !loaded ? (
          <WorkbenchLoading label="正在读取通知" />
        ) : error && !items.length ? (
          <WorkbenchError
            message={error}
            retry={() => void load(status).catch(() => undefined)}
          />
        ) : (
          <>
            {error && (
              <div role="alert" className="wb-state-banner wb-banner-warning">
                <ShieldAlertIcon size={16} />
                <div>
                  <strong>未能更新通知</strong>
                  <span>{error}</span>
                </div>
                <WorkbenchButton
                  variant="outline"
                  onClick={() => void load(status).catch(() => undefined)}
                >
                  重试
                </WorkbenchButton>
              </div>
            )}
            {!shown.length ? (
              loading ? (
                <WorkbenchLoading label="正在筛选通知" />
              ) : (
                <WorkbenchEmpty
                  icon={InboxIcon}
                  title={
                    query
                      ? "没有找到相关消息"
                      : status === "pending"
                        ? "没有需要处理的问题"
                        : "暂时没有通知"
                  }
                  description={
                    query
                      ? "搜索仅覆盖已加载的消息，可以换个关键词或加载更早的通知。"
                      : "有空闲口或连接状态变化时，会在这里留下记录。"
                  }
                  action={
                    <Link className="wb-text-link" href="/dashboard">
                      看看常用桩
                      <ArrowRightIcon size={14} />
                    </Link>
                  }
                />
              )
            ) : (
              <div className="wb-notice-list">
                {shown.map((item) => {
                  const presentation = notificationPresentation(item),
                    requiresAction = notificationRequiresAction(item.type)
                  return (
                    <button
                      className={`wb-notice-row ${!item.readAt ? "wb-unread" : ""}`}
                      key={item.id}
                      onClick={() => void open(item)}
                    >
                      <span
                        className={`wb-notice-icon ${requiresAction && !item.resolvedAt ? "wb-tone-warning" : "wb-tone-idle"}`}
                      >
                        {requiresAction && !item.resolvedAt ? (
                          <ShieldAlertIcon size={19} />
                        ) : (
                          <BellIcon size={19} />
                        )}
                      </span>
                      <span className="wb-notice-main">
                        <span className="wb-notice-title">
                          {presentation.title}
                          {!item.readAt && (
                            <i className="wb-unread-dot" aria-label="未读" />
                          )}
                          {requiresAction && (
                            <StatusPill
                              tone={item.resolvedAt ? "neutral" : "warning"}
                            >
                              {item.resolvedAt ? "已解决" : "待处理"}
                            </StatusPill>
                          )}
                        </span>
                        <span className="wb-notice-message">
                          {presentation.message}
                        </span>
                        <span className="wb-small wb-muted">
                          {item.deviceId
                            ? snapshot.piles.find(
                                (pile) => pile.id === item.deviceId
                              )?.name || "充电桩消息"
                            : "账户连接"}
                          {item.occurrenceCount > 1
                            ? ` · 重复 ${item.occurrenceCount} 次`
                            : ""}
                        </span>
                      </span>
                      <time>
                        {formatSnapshotTime(item.lastOccurredAt, true)}
                      </time>
                      <ChevronRightIcon size={15} />
                    </button>
                  )
                })}
              </div>
            )}
            {nextCursor && (
              <div className="wb-list-pagination">
                <WorkbenchButton
                  variant="outline"
                  busy={loadingMore}
                  disabled={!online}
                  onClick={() =>
                    void loadMore().catch((reason) =>
                      notify.error(reason, { title: "加载更早通知失败" })
                    )
                  }
                >
                  加载更早通知
                </WorkbenchButton>
              </div>
            )}
          </>
        )}
        <Dialog
          open={confirmClear}
          onOpenChange={(open) => !pending && setConfirmClear(open)}
        >
          <DialogContent className="workbench-dialog">
            <DialogHeader>
              <DialogTitle>清理已解决的通知？</DialogTitle>
              <DialogDescription>
                只清理已解决的连接问题，待处理事项和空闲消息会保留。
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <WorkbenchButton
                variant="outline"
                disabled={pending}
                onClick={() => setConfirmClear(false)}
              >
                取消
              </WorkbenchButton>
              <WorkbenchButton
                variant="destructive"
                busy={pending}
                disabled={!online}
                onClick={() => void clear()}
              >
                确认清理
              </WorkbenchButton>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </UserWorkbenchShell>
  )
}
