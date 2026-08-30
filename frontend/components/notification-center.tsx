"use client"

import {
  BellIcon,
  BellOffIcon,
  CheckCheckIcon,
  ChevronRightIcon,
  CircleAlertIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  MapPinIcon,
  Trash2Icon,
  WifiOffIcon,
  ZapIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { notify } from "@/lib/feedback"

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type {
  Notification as AppNotification,
  NotificationStatusFilter,
} from "@/lib/api/generated"
import { useNotifications } from "@/lib/notification-context"
import {
  notificationPresentation,
  notificationRequiresAction,
} from "@/lib/notification-semantics"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import { formatPowerWindow } from "@/lib/watch-format"
import { WxPusherChannelCard } from "@/components/wxpusher-channel-card"
import { LeadingIcon } from "@/components/leading-icon"

const browserSettingLayout =
  "grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 p-4 [&>[data-slot=leading-icon]]:row-span-2"

const typeMeta = {
  pile_available: {
    label: "有空闲充电口",
    icon: ZapIcon,
    tone: "text-success",
    source: "空闲提醒",
    action: "查看空闲口",
    advice: "现在有空闲口，可以前往充电。",
  },
  credential_expired: {
    label: "需要重新登录",
    icon: KeyRoundIcon,
    tone: "text-destructive",
    source: "账户提醒",
    action: "重新扫码",
    advice: "重新扫码后，充电桩状态和空闲提醒会继续更新。",
  },
  pile_offline: {
    label: "充电桩无法连接",
    icon: WifiOffIcon,
    tone: "text-warning-foreground",
    source: "连接提醒",
    action: "查看充电桩",
    advice: "请稍后重试；正常供电后仍无法连接时，可以联系管理员。",
  },
  pile_recovered: {
    label: "充电桩恢复连接",
    icon: CheckCheckIcon,
    tone: "text-success",
    source: "连接提醒",
    action: "查看充电桩",
    advice: "充电桩已经恢复，可以查看最新充电口状态。",
  },
} as const

const statusOptions: Array<{
  value: NotificationStatusFilter
  label: string
}> = [
  { value: "all", label: "全部" },
  { value: "unread", label: "未读" },
  { value: "pending", label: "需处理" },
  { value: "resolved", label: "已恢复" },
]

function formatNotificationTime(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function pileLabel(piles: Pile[], deviceId?: string) {
  if (!deviceId) return null
  const pile = piles.find((candidate) => candidate.id === deviceId)
  return pile?.name || pile?.number || deviceId
}

function BrowserNotificationSetting() {
  const { preference } = useWatch()
  const { browserPermission, requestBrowserPermission, setBrowserEnabled } =
    useNotifications()
  const [pending, setPending] = useState(false)
  const enabled =
    browserPermission === "granted" && Boolean(preference?.browserEnabled)

  async function requestPermission() {
    setPending(true)
    try {
      const permission = await requestBrowserPermission()
      if (permission === "granted") notify.success("浏览器通知已开启")
      else if (permission === "denied")
        notify.warning("通知权限未开启", {
          description: "请在浏览器的网站设置中允许通知。",
          id: "browser-notification-permission",
        })
    } catch (reason) {
      notify.error(reason, { title: "申请通知权限失败" })
    } finally {
      setPending(false)
    }
  }

  async function toggle(next: boolean) {
    setPending(true)
    try {
      const applied = await setBrowserEnabled(next)
      if (next && !applied && browserPermission === "denied")
        notify.warning("通知权限未开启", {
          description: "请先在浏览器的网站设置中允许通知。",
          id: "browser-notification-permission",
        })
      else notify.success(applied ? "浏览器通知已开启" : "浏览器通知已关闭")
    } catch (reason) {
      notify.error(reason, { title: "更新浏览器通知失败" })
    } finally {
      setPending(false)
    }
  }

  if (browserPermission === "unsupported")
    return (
      <Alert className={browserSettingLayout}>
        <LeadingIcon icon={BellOffIcon} />
        <AlertTitle>当前浏览器不支持网页通知</AlertTitle>
        <AlertDescription>
          消息仍会保留在通知中心，可以随时从右上角查看。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "denied")
    return (
      <Alert variant="destructive" className={browserSettingLayout}>
        <LeadingIcon icon={BellOffIcon} className="bg-destructive/10" />
        <AlertTitle>浏览器通知权限已被拒绝</AlertTitle>
        <AlertDescription>
          请在浏览器的网站通知设置中开启权限，消息仍可在通知中心查看。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "default")
    return (
      <Alert className={`${browserSettingLayout} pr-28`}>
        <LeadingIcon icon={BellIcon} className="bg-primary/10 text-primary" />
        <AlertTitle>网页开着时提醒我</AlertTitle>
        <AlertDescription>
          只要此网页保持打开，就会弹出空闲口和账户异常提醒。
        </AlertDescription>
        <div className="absolute top-2 right-2">
          <Button
            size="sm"
            disabled={pending}
            onClick={() => void requestPermission()}
          >
            {pending ? (
              <LoaderCircleIcon className="motion-safe:animate-spin" />
            ) : null}
            允许通知
          </Button>
        </div>
      </Alert>
    )

  return (
    <Alert className={browserSettingLayout}>
      <LeadingIcon icon={BellIcon} className="bg-primary/10 text-primary" />
      <AlertTitle className="flex items-center justify-between gap-3">
        <span>网页提醒</span>
        <Switch
          size="sm"
          aria-label="网页提醒"
          checked={enabled}
          disabled={pending || !preference}
          onCheckedChange={(next) => void toggle(next)}
        />
      </AlertTitle>
      <AlertDescription>
        {preference?.quietHoursEnabled
          ? `${formatPowerWindow(preference.quietStartMinute, preference.quietEndMinute)} 暂停弹窗，消息仍会保留在通知中心。`
          : "网页保持打开时，有空闲口或账户问题会及时提醒你。"}
      </AlertDescription>
    </Alert>
  )
}

function NotificationItem({
  notification,
  piles,
  onNavigate,
}: {
  notification: AppNotification
  piles: Pile[]
  onNavigate: (notification: AppNotification) => void
}) {
  const { markRead } = useNotifications()
  const meta = typeMeta[notification.type]
  const Icon = meta.icon
  const targetPile = pileLabel(piles, notification.deviceId)
  const requiresAction = notificationRequiresAction(notification.type)
  const presentation = notificationPresentation(notification)

  async function openTarget() {
    try {
      if (!notification.readAt) await markRead(notification.id)
      onNavigate(notification)
    } catch (reason) {
      notify.error(reason, {
        title: "暂时无法打开通知",
        description: "请稍后再试，通知仍保留在这里。",
      })
    }
  }

  return (
    <article
      className={`rounded-xl border bg-card transition-colors duration-200 ${
        notification.readAt ? "" : "border-primary/25 bg-primary/3"
      }`}
    >
      <button
        type="button"
        className="grid w-full grid-cols-[auto_minmax(0,1fr)] gap-3 p-4 text-left"
        onClick={() => void openTarget()}
      >
        <LeadingIcon icon={Icon} className={meta.tone} />
        <span className="min-w-0">
          <span className="flex flex-wrap items-center gap-2">
            <strong className="text-sm font-semibold">
              {presentation.title}
            </strong>
            {!notification.readAt ? (
              <span
                className="size-2 rounded-full bg-primary"
                aria-label="未读"
              />
            ) : null}
          </span>
          <span className="mt-1 block text-xs leading-5 text-muted-foreground">
            {presentation.message}
          </span>
          <span className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>来源：{meta.source}</span>
            {targetPile ? (
              <span className="inline-flex items-center gap-1">
                <MapPinIcon className="size-3" />
                {targetPile}
              </span>
            ) : null}
            {notification.portId ? (
              <span>{notification.portId} 号充电口</span>
            ) : null}
          </span>
          <span
            key={`${notification.lastOccurredAt}-${notification.readAt}-${notification.resolvedAt}`}
            className="feedback-status-change mt-2 flex flex-wrap gap-1.5"
          >
            <Badge variant="outline">{meta.label}</Badge>
            {!notification.readAt ? <Badge>未读</Badge> : null}
            {requiresAction ? (
              notification.resolvedAt ? (
                <Badge variant="secondary">已恢复</Badge>
              ) : (
                <Badge variant="outline">需处理</Badge>
              )
            ) : null}
            {notification.occurrenceCount > 1 ? (
              <Badge variant="outline">
                重复 {notification.occurrenceCount} 次
              </Badge>
            ) : null}
          </span>
          <span className="mt-2 block text-xs leading-5 text-muted-foreground">
            发生时间：{formatNotificationTime(notification.createdAt)}
            {notification.occurrenceCount > 1
              ? ` · 最近一次：${formatNotificationTime(notification.lastOccurredAt)}`
              : ""}
          </span>
          <span className="mt-2 block text-xs leading-5">
            建议：{meta.advice}
          </span>
          <span className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary">
            {meta.action}
            <ChevronRightIcon className="size-3" />
          </span>
        </span>
      </button>
    </article>
  )
}

export function NotificationCenter({
  piles,
  onNavigate,
}: {
  piles: Pile[]
  onNavigate: (notification: AppNotification) => void
}) {
  const {
    items,
    unreadCount,
    nextCursor,
    status,
    loading,
    loadingMore,
    loaded,
    error,
    load,
    loadMore,
    markAllRead,
    clearResolved,
  } = useNotifications()
  const [open, setOpen] = useState(false)
  const badgeLabel = useMemo(
    () => (unreadCount > 99 ? "99+" : String(unreadCount)),
    [unreadCount]
  )

  useEffect(() => {
    if (open && !loaded && !loading) void load().catch(() => undefined)
  }, [load, loaded, loading, open])

  async function selectStatus(value: string) {
    try {
      await load(value as NotificationStatusFilter)
    } catch {
      return
    }
  }

  async function readAll() {
    try {
      const updated = await markAllRead()
      notify.success(updated ? "通知已全部标记为已读" : "没有未读通知", {
        description: updated ? `本次更新 ${updated} 条通知。` : undefined,
      })
    } catch (reason) {
      notify.error(reason, { title: "标记通知失败" })
    }
  }

  async function removeResolved() {
    try {
      const deleted = await clearResolved()
      notify.success(deleted ? "已解决通知已清理" : "没有可清理的通知", {
        description: deleted ? `本次清理 ${deleted} 条通知。` : undefined,
      })
    } catch (reason) {
      notify.error(reason, { title: "清理通知失败" })
    }
  }

  return (
    <>
      <Button
        variant="outline"
        className="relative max-md:size-9 max-md:px-0"
        aria-label={unreadCount ? `通知，${unreadCount} 条未读` : "通知"}
        onClick={() => setOpen(true)}
      >
        <BellIcon />
        <span className="max-md:sr-only">通知</span>
        {unreadCount ? (
          <span
            key={unreadCount}
            className="notification-badge-change text-destructive-foreground absolute -top-2 -right-2 grid min-w-5 place-items-center rounded-full bg-destructive px-1 text-[10px] leading-5 font-semibold shadow-sm"
          >
            {badgeLabel}
          </span>
        ) : null}
      </Button>

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          side="right"
          className="data-[side=right]:w-[calc(100vw-0.5rem)] data-[side=right]:max-w-xl data-[side=right]:overflow-y-auto data-[side=right]:p-0 sm:data-[side=right]:w-[min(36rem,calc(100vw-2rem))] sm:data-[side=right]:max-w-xl [&_[data-slot=sheet-close]]:z-20"
        >
          <SheetHeader className="sticky top-0 z-10 border-b bg-popover/95 py-4 pr-14 pl-5 backdrop-blur sm:pl-6">
            <SheetTitle className="flex items-center gap-2">
              通知中心
              {unreadCount ? (
                <Badge variant="destructive">{unreadCount} 条未读</Badge>
              ) : null}
            </SheetTitle>
            <SheetDescription>
              空闲口、重新登录和充电桩连接消息都会保留在这里。
            </SheetDescription>
          </SheetHeader>

          <div className="flex flex-col gap-4 p-4 sm:p-6">
            <BrowserNotificationSetting />

            <WxPusherChannelCard active={open} />

            <div className="grid gap-2">
              <Tabs
                value={status}
                onValueChange={(value) => void selectStatus(value)}
              >
                <TabsList className="grid w-full grid-cols-4">
                  {statusOptions.map((option) => (
                    <TabsTrigger key={option.value} value={option.value}>
                      {option.label}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!unreadCount}
                  onClick={() => void readAll()}
                >
                  <CheckCheckIcon />
                  全部已读
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void removeResolved()}
                >
                  <Trash2Icon />
                  清理已恢复
                </Button>
              </div>
            </div>

            {error ? (
              <Alert urgent variant="destructive" className="pr-24">
                <CircleAlertIcon />
                <AlertTitle>通知加载失败</AlertTitle>
                <AlertDescription>
                  当前无法更新通知，请检查网络后重试。
                  <details className="mt-1 text-xs">
                    <summary className="cursor-pointer">查看详情</summary>
                    <span className="break-words">{error}</span>
                  </details>
                </AlertDescription>
                <AlertAction>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void load()}
                  >
                    重试
                  </Button>
                </AlertAction>
              </Alert>
            ) : null}

            {loading ? (
              <div className="grid gap-3" aria-label="正在加载通知">
                <Skeleton className="h-36" />
                <Skeleton className="h-36" />
              </div>
            ) : items.length ? (
              <div className="grid gap-3" aria-live="polite">
                {items.map((notification) => (
                  <NotificationItem
                    key={notification.id}
                    notification={notification}
                    piles={piles}
                    onNavigate={(target) => {
                      setOpen(false)
                      onNavigate(target)
                    }}
                  />
                ))}
              </div>
            ) : (
              <Empty className="min-h-56 border">
                <EmptyHeader>
                  <EmptyMedia variant="icon">
                    <BellIcon />
                  </EmptyMedia>
                  <EmptyTitle>
                    {status === "unread"
                      ? "没有未读通知"
                      : status === "pending"
                        ? "没有需要处理的通知"
                        : status === "resolved"
                          ? "没有已恢复的问题"
                          : "暂时没有通知"}
                  </EmptyTitle>
                  <EmptyDescription>
                    开启空闲提醒后，有空闲口或账户需要处理时会在这里通知你。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}

            {nextCursor ? (
              <Button
                variant="outline"
                disabled={loadingMore}
                onClick={() =>
                  void loadMore().catch((reason) =>
                    notify.error(reason, {
                      title: "加载更早通知失败",
                      id: "notification-load-more",
                    })
                  )
                }
              >
                {loadingMore ? (
                  <LoaderCircleIcon className="motion-safe:animate-spin" />
                ) : null}
                {loadingMore ? "加载中…" : "加载更早通知"}
              </Button>
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}
