/* eslint-disable @next/next/no-img-element -- WxPusher QR URLs are short-lived and cannot use a stable image loader. */
"use client"

import {
  CheckCircle2Icon,
  CircleAlertIcon,
  LoaderCircleIcon,
  MessageCircleIcon,
  QrCodeIcon,
  UnlinkIcon,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import type {
  WxPusherBindSession,
  WxPusherChannelState,
} from "@/lib/api/generated"
import {
  createWxPusherBindSession,
  deleteWxPusherChannel,
  getWxPusherChannel,
  pollWxPusherBindSession,
} from "@/lib/wxpusher-api"

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
  const [binding, setBinding] = useState(false)
  const [session, setSession] = useState<WxPusherBindSession | null>(null)
  const [bindOpen, setBindOpen] = useState(false)
  const [confirmUnbind, setConfirmUnbind] = useState(false)
  const [unbinding, setUnbinding] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const abortRef = useRef<AbortController | null>(null)
  const pollInFlight = useRef(false)

  const loadChannel = useCallback(async () => {
    setLoading(true)
    try {
      setChannel(await getWxPusherChannel())
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (active && !channel && !loading) {
      const task = window.setTimeout(() => void loadChannel(), 0)
      return () => window.clearTimeout(task)
    }
  }, [active, channel, loadChannel, loading])

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
      toast.error((reason as Error).message)
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
        toast.success("微信提醒绑定成功")
      }
    } catch (reason) {
      if (!controller.signal.aborted) toast.error((reason as Error).message)
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
      toast.success("已解除微信提醒绑定")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setUnbinding(false)
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
      <Alert>
        <MessageCircleIcon />
        <AlertTitle>微信提醒暂未开放</AlertTitle>
        <AlertDescription>站内通知和浏览器提醒仍可正常使用。</AlertDescription>
      </Alert>
    )

  return (
    <>
      <section className="rounded-xl border bg-card p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-success/10 text-success">
              <MessageCircleIcon className="size-4" />
            </span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-semibold">微信提醒</h3>
                <Badge variant={channel.bound ? "secondary" : "outline"}>
                  {channel.bound ? "已绑定" : "未绑定"}
                </Badge>
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {channel.bound
                  ? `${channel.maskedUid ?? "接收账号"} · ${formatBoundAt(channel.boundAt)} 绑定`
                  : "绑定后，离开网页也能收到空闲和异常提醒。"}
              </p>
            </div>
          </div>
          {channel.bound ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirmUnbind(true)}
            >
              <UnlinkIcon />
              解除绑定
            </Button>
          ) : (
            <Button
              size="sm"
              disabled={binding}
              onClick={() => void beginBinding()}
            >
              {binding ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <QrCodeIcon />
              )}
              获取二维码
            </Button>
          )}
        </div>
      </section>

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
              之后不会再向当前微信接收账号发送消息，站内通知不受影响。
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
              {unbinding ? <LoaderCircleIcon className="animate-spin" /> : null}
              解除绑定
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
