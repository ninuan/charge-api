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
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import { formatPowerWindow } from "@/lib/watch-format"

const typeMeta = {
  pile_available: {
    label: "整桩有空闲",
    icon: ZapIcon,
    tone: "text-success",
  },
  credential_expired: {
    label: "凭据失效",
    icon: KeyRoundIcon,
    tone: "text-destructive",
  },
  pile_offline: {
    label: "充电桩离线",
    icon: WifiOffIcon,
    tone: "text-warning-foreground",
  },
  pile_recovered: {
    label: "充电桩恢复",
    icon: CheckCheckIcon,
    tone: "text-success",
  },
} as const

const statusOptions: Array<{
  value: NotificationStatusFilter
  label: string
}> = [
  { value: "all", label: "全部" },
  { value: "unread", label: "未读" },
  { value: "resolved", label: "已解决" },
]

function formatNotificationTime(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
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
      if (permission === "granted") toast.success("浏览器通知已开启")
      else if (permission === "denied") toast.error("浏览器拒绝了通知权限")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setPending(false)
    }
  }

  async function toggle(next: boolean) {
    setPending(true)
    try {
      const applied = await setBrowserEnabled(next)
      if (next && !applied && browserPermission === "denied")
        toast.error("请先在浏览器的网站设置中允许通知")
      else toast.success(applied ? "浏览器通知已开启" : "浏览器通知已关闭")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setPending(false)
    }
  }

  if (browserPermission === "unsupported")
    return (
      <Alert>
        <BellOffIcon />
        <AlertTitle>当前浏览器不支持网页通知</AlertTitle>
        <AlertDescription>
          站内通知仍会完整保存，可以随时从右上角通知中心查看。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "denied")
    return (
      <Alert variant="destructive">
        <BellOffIcon />
        <AlertTitle>浏览器通知权限已被拒绝</AlertTitle>
        <AlertDescription>
          请在浏览器地址栏旁的网站设置中将“通知”改为允许；本页不会反复弹出授权请求。
        </AlertDescription>
      </Alert>
    )

  if (browserPermission === "default")
    return (
      <Alert className="pr-28">
        <BellIcon />
        <AlertTitle>在网页打开时接收即时提醒</AlertTitle>
        <AlertDescription>
          授权后，整桩出现空闲口或发生需处理的异常时可收到浏览器通知。
        </AlertDescription>
        <div className="absolute top-2 right-2">
          <Button
            size="sm"
            disabled={pending}
            onClick={() => void requestPermission()}
          >
            {pending ? <LoaderCircleIcon className="animate-spin" /> : null}
            允许通知
          </Button>
        </div>
      </Alert>
    )

  return (
    <Alert>
      <BellIcon />
      <AlertTitle className="flex items-center justify-between gap-3">
        <span>浏览器即时提醒</span>
        <Switch
          size="sm"
          aria-label="浏览器即时提醒"
          checked={enabled}
          disabled={pending || !preference}
          onCheckedChange={(next) => void toggle(next)}
        />
      </AlertTitle>
      <AlertDescription>
        {preference?.quietHoursEnabled
          ? `${formatPowerWindow(preference.quietStartMinute, preference.quietEndMinute)} 免打扰；站内记录不受影响。`
          : "免打扰已关闭；恢复在线消息仍只保留在站内。"}
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

  async function openTarget() {
    try {
      if (!notification.readAt) await markRead(notification.id)
      onNavigate(notification)
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  return (
    <article
      className={`notification-item-enter rounded-xl border bg-card transition-colors duration-200 ${
        notification.readAt ? "" : "border-primary/25 bg-primary/3"
      }`}
    >
      <button
        type="button"
        className="grid w-full grid-cols-[auto_minmax(0,1fr)_auto] gap-3 p-4 text-left"
        onClick={() => void openTarget()}
      >
        <span
          className={`mt-0.5 grid size-9 place-items-center rounded-lg bg-muted ${meta.tone}`}
        >
          <Icon className="size-4" />
        </span>
        <span className="min-w-0">
          <span className="flex flex-wrap items-center gap-2">
            <strong className="text-sm font-semibold">
              {notification.title}
            </strong>
            {!notification.readAt ? (
              <span
                className="size-2 rounded-full bg-primary"
                aria-label="未读"
              />
            ) : null}
          </span>
          <span className="mt-1 block text-xs leading-5 text-muted-foreground">
            {notification.message}
          </span>
          <span className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{formatNotificationTime(notification.createdAt)}</span>
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
          <span className="mt-2 flex flex-wrap gap-1.5">
            <Badge variant="outline">{meta.label}</Badge>
            {notification.resolvedAt ? (
              <Badge variant="secondary">已解决</Badge>
            ) : null}
          </span>
        </span>
        <ChevronRightIcon className="mt-2 size-4 text-muted-foreground" />
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
    if (open && !loaded && !loading)
      void load().catch((reason) => toast.error((reason as Error).message))
  }, [load, loaded, loading, open])

  async function selectStatus(value: string) {
    try {
      await load(value as NotificationStatusFilter)
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  async function readAll() {
    try {
      const updated = await markAllRead()
      toast.success(
        updated ? `已将 ${updated} 条通知标记为已读` : "没有未读通知"
      )
    } catch (reason) {
      toast.error((reason as Error).message)
    }
  }

  async function removeResolved() {
    try {
      const deleted = await clearResolved()
      toast.success(
        deleted ? `已清理 ${deleted} 条已解决通知` : "没有可清理的通知"
      )
    } catch (reason) {
      toast.error((reason as Error).message)
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
          className="w-[calc(100vw-0.5rem)] max-w-xl overflow-y-auto p-0 sm:w-[min(36rem,calc(100vw-2rem))] sm:max-w-xl [&_[data-slot=sheet-close]]:z-20"
        >
          <SheetHeader className="sticky top-0 z-10 border-b bg-popover/95 py-4 pr-14 pl-5 backdrop-blur sm:pl-6">
            <SheetTitle className="flex items-center gap-2">
              通知中心
              {unreadCount ? (
                <Badge variant="destructive">{unreadCount} 条未读</Badge>
              ) : null}
            </SheetTitle>
            <SheetDescription>
              整桩空闲、凭据和设备异常会持久化保存在这里。
            </SheetDescription>
          </SheetHeader>

          <div className="flex flex-col gap-4 p-4 sm:p-6">
            <BrowserNotificationSetting />

            <div className="grid gap-2">
              <Tabs
                value={status}
                onValueChange={(value) => void selectStatus(value)}
              >
                <TabsList className="grid w-full grid-cols-3">
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
                  清理已解决
                </Button>
              </div>
            </div>

            {error ? (
              <Alert variant="destructive">
                <CircleAlertIcon />
                <AlertTitle>通知加载失败</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
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
                      : status === "resolved"
                        ? "没有已解决通知"
                        : "暂时没有通知"}
                  </EmptyTitle>
                  <EmptyDescription>
                    只有设置整桩空闲提醒后，系统才会按桩号低频检查并记录结果。
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
                    toast.error((reason as Error).message)
                  )
                }
              >
                {loadingMore ? (
                  <LoaderCircleIcon className="animate-spin" />
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
