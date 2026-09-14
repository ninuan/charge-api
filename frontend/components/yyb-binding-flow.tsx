"use client"

/* eslint-disable @next/next/no-img-element -- Short-lived QR data supplied by the scan service. */
import {
  AlertTriangleIcon,
  CheckCircle2Icon,
  LoaderCircleIcon,
  QrCodeIcon,
  RefreshCwIcon,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { useOnline } from "@/lib/browser-state"
import { requestJSON } from "@/lib/http"

import type {
  YybBinding,
  YybQr as QR,
  YybQrPoll as Poll,
} from "@/lib/api/generated"
export type { YybBinding } from "@/lib/api/generated"

const readyStatuses = new Set(["authorized", "confirmed"])
const waitingStatuses = new Set(["pending", "scanned", "confirming"])
const statusText: Record<string, string> = {
  pending: "请用微信扫一扫",
  scanned: "已扫码，请在微信中确认授权",
  authorized: "微信已授权，最后一步：点击“确认绑定”。",
  confirmed: "微信授权已完成，请点击“确认绑定”保存到当前账户。",
  confirming: "正在确认绑定，请稍候…",
  expired: "二维码已过期，请重新生成",
  cancelled: "扫码已取消，请重新生成二维码",
  unknown: "二维码状态无法确认，请重新生成",
}

// A body, not a second Dialog: both entry points use the same session lifecycle.
export function YybBindingFlow({
  onContinue,
  onBack,
  continueLabel = "完成",
}: {
  onContinue: (binding: YybBinding) => void
  onBack?: () => void
  continueLabel?: string
}) {
  const online = useOnline()
  const [qr, setQR] = useState<QR | null>(null)
  const [status, setStatus] = useState("pending")
  const [result, setResult] = useState<YybBinding | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [failures, setFailures] = useState(0)
  const [revision, setRevision] = useState(0)
  const epoch = useRef(0)
  const operation = useRef<AbortController | null>(null)
  const mounted = useRef(false)

  const create = useCallback(async () => {
    const version = ++epoch.current
    operation.current?.abort()
    const controller = new AbortController()
    operation.current = controller
    setQR(null)
    setResult(null)
    setStatus("pending")
    setError("")
    setFailures(0)
    setBusy(false)
    setLoading(true)
    try {
      const next = await requestJSON<QR>(
        "/api/session/yyb-qr",
        { method: "POST", signal: controller.signal },
        "二维码暂时无法生成，请重新尝试。"
      )
      if (!mounted.current || version !== epoch.current) return
      if (!next.sessionId || !(next.imageBase64 || next.imageUrl))
        throw new Error("二维码暂时无法显示，请重新生成。")
      setQR(next)
    } catch (reason) {
      if (mounted.current && version === epoch.current)
        setError(
          reason instanceof Error
            ? reason.message
            : "二维码暂时无法生成，请重新尝试。"
        )
    } finally {
      if (mounted.current && version === epoch.current) {
        operation.current = null
        setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    mounted.current = true
    let cancelled = false
    // Avoid duplicate QR creation during React's development effect replay.
    void Promise.resolve().then(() => {
      if (!cancelled && navigator.onLine) void create()
    })
    return () => {
      cancelled = true
      mounted.current = false
      // Invalidate every in-flight response, including requests started after mount.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      epoch.current++
      operation.current?.abort()
    }
  }, [create])

  useEffect(() => {
    function disconnect() {
      epoch.current++
      operation.current?.abort()
      operation.current = null
      setBusy(false)
      setLoading(false)
      setStatus("pending")
      setError("连接已中断，请恢复连接后重新检查。")
      setFailures(3)
    }
    window.addEventListener("offline", disconnect)
    return () => window.removeEventListener("offline", disconnect)
  }, [])

  const check = useCallback(async () => {
    if (!qr || operation.current || !navigator.onLine) return
    const version = epoch.current
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    try {
      const next = await requestJSON<Poll>(
        `/api/session/yyb-qr/${encodeURIComponent(qr.sessionId)}/poll`,
        { signal: controller.signal, timeoutMs: 45_000 },
        "暂时无法确认扫码结果，请重新检查。"
      )
      if (!mounted.current || version !== epoch.current) return
      if (next.sessionId !== qr.sessionId)
        throw new Error("扫码会话不匹配，请重新检查。")
      if (
        next.status === "saved" &&
        next.binding?.bound &&
        next.binding.sessionId === qr.sessionId
      )
        setResult(next.binding)
      else setStatus(statusText[next.status] ? next.status : "unknown")
      setFailures(0)
      setError("")
    } catch (reason) {
      if (mounted.current && version === epoch.current) {
        setFailures((count) => count + 1)
        setError(
          reason instanceof Error
            ? reason.message
            : "暂时无法确认扫码结果，请重新检查。"
        )
      }
    } finally {
      if (mounted.current && version === epoch.current) {
        operation.current = null
        setBusy(false)
        setRevision((value) => value + 1)
      }
    }
  }, [qr])

  useEffect(() => {
    if (
      !online ||
      !qr ||
      result ||
      loading ||
      busy ||
      failures >= 3 ||
      !waitingStatuses.has(status)
    )
      return
    const timer = setTimeout(() => void check(), 3000)
    return () => clearTimeout(timer)
  }, [online, qr, result, loading, busy, failures, status, revision, check])

  async function confirm() {
    if (
      !qr ||
      !navigator.onLine ||
      operation.current ||
      !readyStatuses.has(status) ||
      error
    )
      return
    const version = epoch.current
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setStatus("confirming")
    try {
      const next = await requestJSON<YybBinding>(
        `/api/session/yyb-qr/${encodeURIComponent(qr.sessionId)}/confirm`,
        { method: "POST", signal: controller.signal, timeoutMs: 120_000 },
        "绑定结果暂时无法确认，正在重新检查。"
      )
      if (!mounted.current || version !== epoch.current) return
      if (!next.bound || (next.sessionId && next.sessionId !== qr.sessionId))
        throw new Error("绑定尚未保存，请重新检查。")
      setResult(next)
      setError("")
    } catch {
      if (mounted.current && version === epoch.current) {
        // Keep confirmation disabled until this exact session is reconciled.
        setError("绑定结果暂时无法确认，请重新检查；不会重复提交。")
        setFailures(0)
      }
    } finally {
      if (mounted.current && version === epoch.current) {
        operation.current = null
        setBusy(false)
        setRevision((value) => value + 1)
      }
    }
  }

  const canConfirm =
    online && !busy && !loading && !error && readyStatuses.has(status)
  const warning = result?.syncState === "failed"
  return (
    <div className="min-w-0 space-y-4">
      {result ? (
        <div className="space-y-4 border-t pt-5">
          <div role="status" className="flex gap-3">
            {warning ? (
              <AlertTriangleIcon className="mt-0.5 size-5 shrink-0 text-warning-foreground" />
            ) : (
              <CheckCircle2Icon className="mt-0.5 size-5 shrink-0 text-success" />
            )}
            <div className="space-y-2">
              <p className="font-semibold">微信已绑定</p>
              <p className="text-sm text-muted-foreground">
                {warning
                  ? "但平台连接暂未恢复。可在账户与设置中重试同步。"
                  : result.cookieSynced
                    ? "平台连接已更新。"
                    : "接下来添加你的常用充电桩。"}
              </p>
            </div>
          </div>
          <Button className="w-full" onClick={() => onContinue(result)}>
            {continueLabel}
          </Button>
        </div>
      ) : (
        <>
          <div className="flex justify-center border-y py-4">
            {qr ? (
              <img
                src={qr.imageBase64 || qr.imageUrl}
                alt="微信扫码登录二维码"
                className="aspect-square w-48 max-w-full bg-white p-2"
              />
            ) : (
              <div className="flex size-48 flex-col items-center justify-center gap-3 text-muted-foreground">
                {loading ? (
                  <LoaderCircleIcon className="size-8 motion-safe:animate-spin" />
                ) : (
                  <QrCodeIcon className="size-8" />
                )}
                <span>{loading ? "正在生成二维码…" : "二维码尚未生成"}</span>
              </div>
            )}
          </div>
          <div aria-live="polite" aria-atomic="true" className="min-h-12">
            <p
              className={
                canConfirm ? "font-semibold text-primary" : "font-medium"
              }
            >
              {!online
                ? "当前离线，请恢复连接后重新检查"
                : loading
                  ? "正在准备扫码"
                  : statusText[status] || statusText.unknown}
            </p>
            {failures >= 3 && (
              <p className="mt-1 text-sm text-muted-foreground">
                自动检测已暂停，请点击“检查扫码状态”重试。
              </p>
            )}
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <Button
            className="w-full"
            disabled={!canConfirm}
            onClick={() => void confirm()}
          >
            {busy && status === "confirming" ? (
              <LoaderCircleIcon className="motion-safe:animate-spin" />
            ) : (
              <CheckCircle2Icon />
            )}
            {busy && status === "confirming" ? "正在确认绑定…" : "确认绑定"}
          </Button>
          <p className="text-sm leading-6 text-muted-foreground">
            微信授权后，还需点击“确认绑定”，才能保存到当前账户。
          </p>
          <div className="grid grid-cols-2 gap-2">
            <Button
              variant="outline"
              disabled={!online || loading || (busy && status === "confirming")}
              onClick={() => void create()}
            >
              重新生成
            </Button>
            <Button
              variant="outline"
              disabled={!online || !qr || busy || loading}
              onClick={() => void check()}
            >
              <RefreshCwIcon
                className={busy ? "motion-safe:animate-spin" : ""}
              />
              检查扫码状态
            </Button>
          </div>
          {onBack && (
            <Button
              variant="ghost"
              disabled={status === "confirming"}
              onClick={onBack}
            >
              返回填写
            </Button>
          )}
          <p className="border-t pt-3 text-xs leading-5 text-muted-foreground">
            手机端请在另一台设备打开此二维码，再用微信扫一扫。当前不支持长按、截图或相册识别。
          </p>
        </>
      )}
    </div>
  )
}
