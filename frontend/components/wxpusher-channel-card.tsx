/* eslint-disable @next/next/no-img-element -- WxPusher QR URLs are short-lived and cannot use a stable image loader. */
"use client"

import {
  CheckCircle2Icon,
  CircleAlertIcon,
  Clock3Icon,
  LoaderCircleIcon,
  MessageCircleIcon,
  QrCodeIcon,
  RefreshCwIcon,
  SendIcon,
  UnlinkIcon,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { notify } from "@/lib/feedback"
import { LeadingIcon } from "@/components/leading-icon"
import {
  deliveryPresentationMessage,
  deliveryPresentationState,
} from "@/lib/notification-semantics"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
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
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Switch } from "@/components/ui/switch"
import type {
  NotificationDeliverySummary,
  WxPusherBindSession,
  WxPusherChannelState,
  WxPusherEventType,
} from "@/lib/api/generated"
import {
  createWxPusherBindSession,
  deleteWxPusherChannel,
  getWxPusherChannel,
  pollWxPusherBindSession,
  recheckWxPusherTestDelivery,
  testWxPusherChannel,
  updateWxPusherChannel,
} from "@/lib/wxpusher-api"
import { cn } from "@/lib/utils"

const eventOptions: Array<{
  value: WxPusherEventType
  label: string
  description: string
}> = [
  {
    value: "pile_available",
    label: "有空闲充电口",
    description: "你正在等待的充电桩出现空闲口时通知你。",
  },
  {
    value: "credential_expired",
    label: "需要重新登录",
    description: "登录过期、需要重新扫码时通知你。",
  },
  {
    value: "pile_offline",
    label: "充电桩无法连接",
    description: "在正常供电时段内，多次无法连接充电桩时通知你。",
  },
  {
    value: "pile_recovered",
    label: "充电桩恢复连接",
    description: "此前无法连接的充电桩恢复后通知你。",
  },
]

const terminalDeliveryStatuses = new Set([
  "provider_succeeded",
  "suppressed",
  "uncertain",
  "failed",
  "cancelled",
])

function deliverySuggestion(delivery?: NotificationDeliverySummary) {
  if (!delivery) return "发送一条测试消息，确认手机能收到微信提醒。"
  switch (deliveryPresentationState(delivery)) {
    case "queued":
      return "测试消息正在发送，请稍候。"
    case "submitted":
      return "消息已交给 WxPusher；暂未取得进一步状态，这不代表发送失败。"
    case "processed":
      return "消息已发送，请检查 WxPusher App 或微信。"
    case "suppressed":
      return "当前处于免打扰时段，本次没有发送到微信，消息已保留在通知中心。"
    case "cancelled":
      return "本次没有发送到微信。"
    case "unknown":
      return "暂时无法确认是否已发送，请稍后重新查询。"
    case "failed":
      break
  }
  switch (delivery.errorCode) {
    case "invalid_uid":
    case "recipient_rejected":
      return "当前接收账号可能已取消关注，请解除绑定后重新扫码。"
    case "invalid_token":
      return "服务配置暂时不可用，请联系管理员检查 WxPusher 配置。"
    case "provider_unavailable":
    case "timeout":
    case "rate_limited":
      return delivery.isTest
        ? "WxPusher 暂时繁忙，可以稍后重新发送测试消息。"
        : "微信提醒暂时繁忙，消息仍会保留在通知中心。"
    case "ambiguous_result":
    case "invalid_response":
      return "暂时无法确认是否已发送，请先检查手机，避免重复发送。"
    default:
      return delivery.isTest
        ? "请检查 WxPusher App 和微信是否收到，稍后也可以重新发送。"
        : "微信提醒没有发出，消息仍会保留在通知中心。"
  }
}

function secondsUntil(value: string | undefined, now: number) {
  if (!value) return 0
  return Math.max(0, Math.ceil((new Date(value).getTime() - now) / 1000))
}

function formatCountdown(seconds: number) {
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return `${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
}

function formatBoundAt(value?: string) {
  if (!value) return ""
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function WxPusherChannelCard({ active }: { active: boolean }) {
  const [channel, setChannel] = useState<WxPusherChannelState | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [binding, setBinding] = useState(false)
  const [session, setSession] = useState<WxPusherBindSession | null>(null)
  const [bindOpen, setBindOpen] = useState(false)
  const [confirmUnbind, setConfirmUnbind] = useState(false)
  const [unbinding, setUnbinding] = useState(false)
  const [pendingSetting, setPendingSetting] = useState<string | null>(null)
  const [testing, setTesting] = useState(false)
  const [rechecking, setRechecking] = useState<string | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const abortRef = useRef<AbortController | null>(null)
  const pollInFlight = useRef(false)
  const channelLoadInFlight = useRef(false)
  const hasPendingDelivery =
    !loadFailed &&
    [channel?.lastDelivery?.status, channel?.lastTestDelivery?.status].some(
      (status) => status && !terminalDeliveryStatuses.has(status)
    )

  const loadChannel = useCallback(async () => {
    if (channelLoadInFlight.current) return
    channelLoadInFlight.current = true
    setLoading(true)
    try {
      setChannel(await getWxPusherChannel())
      setLoadFailed(false)
    } catch {
      setLoadFailed(true)
    } finally {
      channelLoadInFlight.current = false
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (active && !channel && !loading && !loadFailed) {
      const task = window.setTimeout(() => void loadChannel(), 0)
      return () => window.clearTimeout(task)
    }
  }, [active, channel, loadChannel, loadFailed, loading])

  useEffect(() => {
    if (!active || !hasPendingDelivery) return

    let disposed = false
    let timer: number | undefined
    let refreshing = false

    const schedule = () => {
      if (disposed || document.visibilityState !== "visible") return
      if (timer !== undefined) window.clearTimeout(timer)
      timer = window.setTimeout(() => void refresh(), 10_000)
    }
    const refresh = async () => {
      if (disposed || refreshing) return
      refreshing = true
      try {
        await loadChannel()
      } finally {
        refreshing = false
        schedule()
      }
    }
    const handleVisibilityChange = () => {
      if (timer !== undefined) {
        window.clearTimeout(timer)
        timer = undefined
      }
      if (document.visibilityState === "visible") void refresh()
    }

    document.addEventListener("visibilitychange", handleVisibilityChange)
    schedule()
    return () => {
      disposed = true
      if (timer !== undefined) window.clearTimeout(timer)
      document.removeEventListener("visibilitychange", handleVisibilityChange)
    }
  }, [active, hasPendingDelivery, loadChannel])

  const stopPolling = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    pollInFlight.current = false
  }, [])

  function changeBindOpen(next: boolean) {
    setBindOpen(next)
    if (!next) stopPolling()
  }

  async function beginBinding() {
    if (
      session?.status === "waiting_scan" &&
      new Date(session.expiresAt).getTime() > Date.now()
    ) {
      setNow(Date.now())
      setBindOpen(true)
      return
    }
    setBinding(true)
    try {
      const created = await createWxPusherBindSession()
      setNow(Date.now())
      setSession(created)
      setBindOpen(true)
    } catch (reason) {
      notify.error(reason, { title: "获取绑定二维码失败" })
    } finally {
      setBinding(false)
    }
  }

  const poll = useCallback(async () => {
    if (!session || session.status !== "waiting_scan" || pollInFlight.current)
      return
    pollInFlight.current = true
    const controller = new AbortController()
    abortRef.current = controller
    try {
      const updated = await pollWxPusherBindSession(session.id, {
        signal: controller.signal,
      })
      setSession(updated)
      setNow(Date.now())
      if (updated.status === "bound") {
        stopPolling()
        await loadChannel()
        notify.success("微信提醒绑定成功", {
          id: "wxpusher-binding-poll",
        })
      }
    } catch (reason) {
      if (!controller.signal.aborted)
        notify.error(reason, {
          title: "查询绑定状态失败",
          id: "wxpusher-binding-poll",
        })
    } finally {
      if (abortRef.current === controller) abortRef.current = null
      pollInFlight.current = false
    }
  }, [loadChannel, session, stopPolling])

  useEffect(() => {
    if (!bindOpen || session?.status !== "waiting_scan") return
    const timer = window.setInterval(() => {
      const currentTime = Date.now()
      setNow(currentTime)
      if (new Date(session.expiresAt).getTime() <= currentTime) {
        stopPolling()
        setSession((current) =>
          current
            ? {
                ...current,
                status: "expired",
                message: "二维码已过期，请重新获取。",
              }
            : current
        )
      }
    }, 1_000)
    return () => window.clearInterval(timer)
  }, [bindOpen, session?.expiresAt, session?.status, stopPolling])

  const expiresIn = secondsUntil(session?.expiresAt, now)
  const nextPollIn = secondsUntil(session?.nextPollAt, now)

  useEffect(() => {
    if (!bindOpen || session?.status !== "waiting_scan") return
    if (expiresIn > 0 && nextPollIn <= 0) {
      const task = window.setTimeout(() => void poll(), 0)
      return () => window.clearTimeout(task)
    }
  }, [bindOpen, expiresIn, nextPollIn, poll, session?.status])

  useEffect(() => stopPolling, [stopPolling])

  async function unbind() {
    setUnbinding(true)
    try {
      await deleteWxPusherChannel()
      setConfirmUnbind(false)
      setChannel((current) =>
        current
          ? {
              ...current,
              bound: false,
              enabled: false,
              maskedUid: undefined,
              boundAt: undefined,
            }
          : current
      )
      notify.success("微信提醒绑定已解除")
    } catch (reason) {
      notify.error(reason, { title: "解除微信绑定失败" })
    } finally {
      setUnbinding(false)
    }
  }

  async function setEnabled(enabled: boolean) {
    setPendingSetting("enabled")
    try {
      const updated = await updateWxPusherChannel({ enabled })
      setChannel(updated)
      notify.success(enabled ? "微信提醒已开启" : "微信提醒已关闭")
    } catch (reason) {
      notify.error(reason, { title: "更新微信提醒失败" })
    } finally {
      setPendingSetting(null)
    }
  }

  async function toggleEvent(eventType: WxPusherEventType, enabled: boolean) {
    if (!channel) return
    setPendingSetting(eventType)
    const selected = new Set(channel.eventTypes)
    if (enabled) selected.add(eventType)
    else selected.delete(eventType)
    try {
      const updated = await updateWxPusherChannel({
        eventTypes: eventOptions
          .map((option) => option.value)
          .filter((value) => selected.has(value)),
      })
      setChannel(updated)
      notify.success("微信接收内容已更新")
    } catch (reason) {
      notify.error(reason, { title: "更新接收内容失败" })
    } finally {
      setPendingSetting(null)
    }
  }

  async function sendTest() {
    setTesting(true)
    try {
      const delivery = await testWxPusherChannel()
      setChannel((current) =>
        current ? { ...current, lastTestDelivery: delivery } : current
      )
      notify.success("测试消息正在发送", {
        description: "发送后请检查 WxPusher App 或微信。",
        id: "wxpusher-test-delivery",
      })
    } catch (reason) {
      notify.error(reason, {
        title: "发送测试消息失败",
        id: "wxpusher-test-delivery",
      })
    } finally {
      setTesting(false)
    }
  }

  async function recheckTest(deliveryId: string) {
    setRechecking(deliveryId)
    try {
      const delivery = await recheckWxPusherTestDelivery()
      setChannel((current) =>
        current ? { ...current, lastTestDelivery: delivery } : current
      )
      const state = deliveryPresentationState(delivery)
      if (state === "processed") {
        notify.success("测试消息已发送，请检查手机", {
          id: "wxpusher-test-delivery",
        })
      } else if (state === "failed") {
        notify.error("请稍后重新发送；消息没有发出，不会重复收到。", {
          title: "测试消息没有发出",
          id: "wxpusher-test-delivery",
        })
      } else if (state === "submitted") {
        notify.success("测试消息已发送，请检查手机", {
          description: "请在 WxPusher App 或微信中确认是否收到。",
          id: "wxpusher-test-delivery",
        })
      } else {
        notify.info("暂时无法确认是否已发送", {
          description: "请先检查手机，稍后也可以重新查询。",
          id: "wxpusher-test-delivery",
        })
      }
    } catch (reason) {
      notify.error(reason, {
        title: "查询测试消息状态失败",
        id: "wxpusher-test-delivery",
      })
    } finally {
      setRechecking(null)
    }
  }

  if (loading && !channel)
    return (
      <Skeleton className="h-32 rounded-xl" aria-label="正在加载微信提醒" />
    )

  if (!channel)
    return (
      <Alert variant="destructive">
        <CircleAlertIcon />
        <AlertTitle>微信提醒状态加载失败</AlertTitle>
        <AlertDescription>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void loadChannel()}
          >
            重新加载
          </Button>
        </AlertDescription>
      </Alert>
    )

  if (!channel.configured)
    return (
      <Alert className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 p-4 [&>[data-slot=leading-icon]]:row-span-2">
        <LeadingIcon
          icon={MessageCircleIcon}
          className="bg-success/10 text-success"
          iconClassName="size-[17px]"
        />
        <AlertTitle>微信提醒暂未开放</AlertTitle>
        <AlertDescription>通知中心和网页提醒仍可正常使用。</AlertDescription>
      </Alert>
    )

  const recentDeliveries = [
    { label: "最近测试", delivery: channel.lastTestDelivery },
    { label: "最近一次发送", delivery: channel.lastDelivery },
  ].filter(
    (item): item is { label: string; delivery: NotificationDeliverySummary } =>
      Boolean(item.delivery)
  )

  return (
    <>
      {loadFailed ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>微信提醒状态更新失败</AlertTitle>
          <AlertDescription>
            当前显示的是上次结果。
            <Button
              variant="outline"
              size="xs"
              className="ml-2"
              disabled={loading}
              onClick={() => void loadChannel()}
            >
              重新加载
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader>
          <div className="flex min-w-0 gap-3">
            <LeadingIcon
              icon={MessageCircleIcon}
              className="bg-success/10 text-success"
              iconClassName="size-[17px]"
            />
            <div className="min-w-0">
              <CardTitle className="flex flex-wrap items-center gap-2">
                <span>微信提醒</span>
                <Badge variant={channel.bound ? "secondary" : "outline"}>
                  {channel.bound ? "已绑定" : "未绑定"}
                </Badge>
              </CardTitle>
              <CardDescription className="mt-1 text-xs leading-5">
                {channel.bound
                  ? `${channel.maskedUid ?? "接收账号"} · ${formatBoundAt(channel.boundAt)} 绑定`
                  : "绑定后，离开网页也能收到空闲和异常提醒。"}
              </CardDescription>
            </div>
          </div>
          <CardAction>
            {channel.bound ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setConfirmUnbind(true)}
              >
                <UnlinkIcon data-icon="inline-start" />
                解除绑定
              </Button>
            ) : (
              <Button
                size="sm"
                disabled={binding}
                onClick={() => void beginBinding()}
              >
                {binding ? (
                  <LoaderCircleIcon
                    data-icon="inline-start"
                    className="motion-safe:animate-spin"
                  />
                ) : (
                  <QrCodeIcon data-icon="inline-start" />
                )}
                获取二维码
              </Button>
            )}
          </CardAction>
        </CardHeader>

        {channel.bound ? (
          <CardContent>
            <FieldGroup>
              <Field
                orientation="horizontal"
                className="rounded-lg border p-3"
                data-disabled={pendingSetting === "enabled"}
              >
                <FieldContent>
                  <FieldLabel htmlFor="wxpusher-enabled">微信提醒</FieldLabel>
                  <FieldDescription>
                    关闭后不再发到微信，消息仍会保留在通知中心。
                  </FieldDescription>
                </FieldContent>
                <Switch
                  id="wxpusher-enabled"
                  checked={channel.enabled}
                  disabled={pendingSetting !== null}
                  onCheckedChange={(enabled) => void setEnabled(enabled)}
                />
              </Field>

              <div>
                <p className="mb-3 text-sm font-medium">接收内容</p>
                <FieldGroup className="grid gap-3 sm:grid-cols-2">
                  {eventOptions.map((option) => {
                    const id = `wxpusher-${option.value}`
                    return (
                      <Field
                        key={option.value}
                        orientation="horizontal"
                        className="rounded-lg border p-3"
                        data-disabled={!channel.enabled}
                      >
                        <FieldContent>
                          <FieldLabel htmlFor={id}>{option.label}</FieldLabel>
                          <FieldDescription>
                            {option.description}
                          </FieldDescription>
                        </FieldContent>
                        <Switch
                          id={id}
                          size="sm"
                          checked={channel.eventTypes.includes(option.value)}
                          disabled={!channel.enabled || pendingSetting !== null}
                          onCheckedChange={(enabled) =>
                            void toggleEvent(option.value, enabled)
                          }
                        />
                      </Field>
                    )
                  })}
                </FieldGroup>
              </div>

              {recentDeliveries.map(({ label, delivery }) => {
                const presentationState = deliveryPresentationState(delivery)
                const failed = presentationState === "failed"
                const unknown = presentationState === "unknown"
                const activelySending = presentationState === "queued"
                const waitingProvider = delivery.status === "accepted"
                const canRecheck =
                  delivery.isTest &&
                  (delivery.status === "accepted" ||
                    delivery.status === "uncertain")
                const canResend = delivery.isTest && failed
                const submittedAt =
                  delivery.acceptedAt ??
                  delivery.createdAt ??
                  delivery.updatedAt
                return (
                  <Alert
                    key={delivery.id}
                    variant={failed ? "destructive" : "default"}
                    className={cn(
                      "feedback-status-card",
                      unknown && "border-amber-500/30 bg-amber-500/5",
                      presentationState === "processed" &&
                        "feedback-success-once"
                    )}
                  >
                    {activelySending ? (
                      <LoaderCircleIcon className="motion-safe:animate-spin" />
                    ) : waitingProvider ? (
                      <Clock3Icon />
                    ) : failed ? (
                      <CircleAlertIcon />
                    ) : unknown || presentationState === "cancelled" ? (
                      <CircleAlertIcon className="text-amber-600" />
                    ) : (
                      <CheckCircle2Icon />
                    )}
                    <AlertTitle
                      key={`${delivery.id}-${delivery.status}`}
                      className="feedback-status-change"
                    >
                      {label} · {deliveryPresentationMessage(delivery)}
                    </AlertTitle>
                    <AlertDescription className="grid gap-2 text-balance">
                      <p>{deliverySuggestion(delivery)}</p>
                      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums">
                        <span>发送 {formatBoundAt(submittedAt)}</span>
                        <span>
                          最近检查 {formatBoundAt(delivery.updatedAt)}
                        </span>
                      </div>
                      {canRecheck || canResend ? (
                        <div className="flex flex-wrap gap-2 pt-0.5">
                          {canRecheck ? (
                            <Button
                              variant="outline"
                              size="xs"
                              disabled={rechecking !== null}
                              onClick={() => void recheckTest(delivery.id)}
                            >
                              <RefreshCwIcon
                                data-icon="inline-start"
                                className={
                                  rechecking === delivery.id
                                    ? "motion-safe:animate-spin"
                                    : undefined
                                }
                              />
                              {rechecking === delivery.id
                                ? "正在查询…"
                                : "重新查询状态"}
                            </Button>
                          ) : null}
                          {canResend ? (
                            <Button
                              variant="outline"
                              size="xs"
                              disabled={testing}
                              onClick={() => void sendTest()}
                            >
                              <SendIcon data-icon="inline-start" />
                              {testing ? "正在提交…" : "重新发送"}
                            </Button>
                          ) : null}
                        </div>
                      ) : null}
                    </AlertDescription>
                  </Alert>
                )
              })}
            </FieldGroup>
          </CardContent>
        ) : null}

        {channel.bound ? (
          <CardFooter className="flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs leading-5 text-muted-foreground">
              {channel.deliveryDisclaimer}
            </p>
            <Button
              variant="outline"
              className="shrink-0"
              disabled={!channel.enabled || testing}
              onClick={() => void sendTest()}
            >
              {testing ? (
                <LoaderCircleIcon
                  data-icon="inline-start"
                  className="motion-safe:animate-spin"
                />
              ) : (
                <SendIcon data-icon="inline-start" />
              )}
              {testing ? "正在提交…" : "发送测试消息"}
            </Button>
          </CardFooter>
        ) : null}
      </Card>

      <Dialog open={bindOpen} onOpenChange={changeBindOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>绑定微信提醒</DialogTitle>
            <DialogDescription>
              用微信扫码并关注，完成后页面会自动确认。
            </DialogDescription>
          </DialogHeader>

          {session?.status === "waiting_scan" ? (
            <div className="grid gap-4">
              <div className="mx-auto overflow-hidden rounded-xl border bg-white p-2 shadow-sm">
                <img
                  src={session.qrUrl}
                  alt="WxPusher 微信提醒绑定二维码"
                  className="size-52 object-contain"
                />
              </div>
              <ol className="grid gap-2 text-sm">
                <li>
                  <strong>1.</strong> 使用微信扫描二维码
                </li>
                <li>
                  <strong>2.</strong> 在打开的页面中确认关注
                </li>
                <li>
                  <strong>3.</strong> 返回这里等待自动完成
                </li>
              </ol>
              <div
                className="flex items-center justify-between rounded-lg bg-muted px-3 py-2 text-xs text-muted-foreground"
                aria-live="polite"
              >
                <span>二维码剩余 {formatCountdown(expiresIn)}</span>
                <span>
                  {nextPollIn ? `${nextPollIn} 秒后确认` : "正在确认…"}
                </span>
              </div>
            </div>
          ) : session?.status === "bound" ? (
            <Alert className="wxpusher-bind-success border-success/30 bg-success/5 text-success">
              <CheckCircle2Icon />
              <AlertTitle>微信提醒已绑定</AlertTitle>
              <AlertDescription>
                之后的空闲和异常消息可发送到微信。
              </AlertDescription>
            </Alert>
          ) : (
            <Alert variant="destructive">
              <CircleAlertIcon />
              <AlertTitle>
                {session?.status === "expired" ? "二维码已过期" : "绑定未完成"}
              </AlertTitle>
              <AlertDescription>
                {session?.message ?? "请关闭后重新获取二维码。"}
              </AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            {session?.status !== "waiting_scan" ? (
              <Button variant="outline" onClick={() => changeBindOpen(false)}>
                关闭
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={confirmUnbind} onOpenChange={setConfirmUnbind}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>解除微信提醒绑定？</DialogTitle>
            <DialogDescription>
              之后不会再向当前微信接收账号发送消息，历史消息仍会保留在通知中心。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmUnbind(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={unbinding}
              onClick={() => void unbind()}
            >
              {unbinding ? (
                <LoaderCircleIcon className="motion-safe:animate-spin" />
              ) : null}
              解除绑定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
