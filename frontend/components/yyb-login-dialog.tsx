"use client"

import { QrCodeIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { useCloseAppShellMenu } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { ManualCookieForm } from "@/components/manual-cookie-form"
import { YybBindingFlow, type YybBinding } from "@/components/yyb-binding-flow"
import { useOnline } from "@/lib/browser-state"
import { requestJSON } from "@/lib/http"

export function YybLoginDialog({
  open: controlledOpen,
  onOpenChange,
}: { open?: boolean; onOpenChange?: (open: boolean) => void } = {}) {
  const closeMenu = useCloseAppShellMenu()
  const [localOpen, setLocalOpen] = useState(false)
  const open = controlledOpen ?? localOpen
  function change(next: boolean) {
    ;(onOpenChange ?? setLocalOpen)(next)
    if (!next) closeMenu()
  }
  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogTrigger
        render={
          <Button variant="outline">
            <QrCodeIcon />
            扫码登录
          </Button>
        }
      />
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
        <DialogHeader className="pr-7">
          <DialogTitle>绑定平台微信</DialogTitle>
          <DialogDescription>
            微信扫码授权后，点击确认绑定，保存到当前账户。
          </DialogDescription>
        </DialogHeader>
        {open && <BindingSettings onDone={() => change(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function BindingSettings({ onDone }: { onDone: () => void }) {
  const online = useOnline()
  const [binding, setBinding] = useState<YybBinding | null>(null)
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)
  const [scan, setScan] = useState(false)
  const [manual, setManual] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    requestJSON<YybBinding>(
      "/api/session/yyb-binding",
      { signal: controller.signal },
      "暂时无法检查平台连接。"
    )
      .then((next) => {
        if (!controller.signal.aborted) {
          setBinding(next)
          setError("")
          setScan(next.scanEnabled !== false && !next.bound)
        }
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setError("暂时无法检查平台连接，请重试。")
      })
    return () => controller.abort()
  }, [retry])
  if ((!online && !binding) || error)
    return (
      <div role="alert" className="space-y-3">
        <p>{!online ? "当前离线，请恢复连接后重试。" : error}</p>
        <Button
          disabled={!online}
          onClick={() => {
            setError("")
            setRetry((value) => value + 1)
          }}
        >
          重试
        </Button>
      </div>
    )
  if (!binding) return <p role="status">正在检查平台连接…</p>
  return (
    <div className="space-y-4">
      {scan ? (
        <YybBindingFlow onContinue={onDone} />
      ) : (
        <>
          <p>
            {binding.bound
              ? `${binding.nickname || "微信账号"} 已绑定`
              : "当前部署未启用扫码，请手动设置 Cookie。"}
          </p>
          {binding.scanEnabled !== false && (
            <Button disabled={!online} onClick={() => setScan(true)}>
              重新扫码绑定
            </Button>
          )}
        </>
      )}
      <Button variant="ghost" onClick={() => setManual((value) => !value)}>
        {manual ? "收起手动设置" : "手动设置 Cookie"}
      </Button>
      {(manual || binding.scanEnabled === false) && <ManualCookieForm />}
    </div>
  )
}
