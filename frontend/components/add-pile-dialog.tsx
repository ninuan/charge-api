"use client"

import { LoaderCircleIcon, PlusIcon, ServerCogIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import dynamic from "next/dynamic"
import type { YybBinding } from "@/components/yyb-binding-flow"

import { requestJSON, RequestError } from "@/lib/http"
import { useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"

import { useCloseAppShellMenu } from "@/components/app-shell"
import { WorkbenchButton as Button } from "@/components/workbench/surfaces"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import { useDashboard } from "@/lib/dashboard-context"
import type { Pile } from "@/lib/types"

const YybBindingFlow = dynamic(
  () => import("@/components/yyb-binding-flow").then((m) => m.YybBindingFlow),
  { ssr: false, loading: () => <p role="status">正在打开绑定…</p> }
)
const ManualCookieForm = dynamic(
  () =>
    import("@/components/manual-cookie-form").then((m) => m.ManualCookieForm),
  { ssr: false, loading: () => <p role="status">正在打开凭据表单…</p> }
)

export function AddPileDialog({
  onAdded,
  disabled = false,
}: { onAdded?: (pile: Pile) => void; disabled?: boolean } = {}) {
  const { currentUser } = useAuth()
  const { addPile } = useDashboard()
  const closeAppShellMenu = useCloseAppShellMenu()
  const [open, setOpen] = useState(false)
  const online = useOnline()
  const [step, setStep] = useState<"checking" | "form" | "binding" | "error">(
    "checking"
  )
  const [binding, setBinding] = useState<YybBinding | null>(null)
  const [bindingRequired, setBindingRequired] = useState(false)
  const [error, setError] = useState("")
  const [retry, setRetry] = useState(0)
  const [manual, setManual] = useState(false)
  const submitLock = useRef(false)
  const numberInput = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    requestJSON<YybBinding>(
      "/api/session/yyb-binding",
      { signal: controller.signal },
      "暂时无法检查平台连接，请重试。"
    )
      .then((next) => {
        if (controller.signal.aborted) return
        const required =
          next.scanEnabled === true &&
          (!next.bound || ["expired", "unknown"].includes(next.status ?? ""))
        setBinding(next)
        setBindingRequired(required)
        setStep(required ? "binding" : "form")
        setError("")
      })
      .catch((reason) => {
        if (controller.signal.aborted) return
        setError(
          reason instanceof Error
            ? reason.message
            : "暂时无法检查平台连接，请重试。"
        )
        setStep("error")
      })
    return () => controller.abort()
  }, [open, retry])
  useEffect(() => {
    if (open && step === "form") numberInput.current?.focus()
  }, [open, step])
  const [advanced, setAdvanced] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({
    id: "",
    name: "",
    number: "",
    openNum: 10,
    status: "在线",
    address: "",
  })
  const change = (name: keyof typeof form, value: string | number) =>
    setForm((current) => ({ ...current, [name]: value }))
  const handleOpenChange = (next: boolean) => {
    if (next) {
      setStep("checking")
      setError("")
    }
    setOpen(next)
    if (!next) closeAppShellMenu()
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitLock.current || bindingRequired || !navigator.onLine) return
    submitLock.current = true
    setError("")
    const id = form.id.trim()
    const number = form.number.trim()
    setSubmitting(true)
    try {
      const added = await addPile({
        id,
        name: form.name.trim() || `充电桩 ${number || id.slice(-6)}`,
        number,
        openNum: Number(form.openNum),
        status: form.status.trim() || "在线",
        address: form.address.trim(),
      })
      setForm({
        id: "",
        name: "",
        number: "",
        openNum: 10,
        status: "在线",
        address: "",
      })
      handleOpenChange(false)
      notify.success("充电桩已添加")
      onAdded?.(added)
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "添加充电桩失败，请稍后重试。"
      )
      if (
        reason instanceof RequestError &&
        ["YYB_BINDING_REQUIRED", "YYB_RESCAN_REQUIRED"].includes(
          reason.code ?? ""
        )
      )
        setBindingRequired(true)
    } finally {
      submitLock.current = false
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button disabled={disabled}>
            <PlusIcon />
            添加充电桩
          </Button>
        }
      />
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader className="pr-7">
          <DialogTitle className="flex items-center gap-2">
            <ServerCogIcon className="size-5" />
            {step === "binding" ? "添加前，先绑定微信" : "添加充电桩"}
          </DialogTitle>
          <DialogDescription>
            当前账户最多添加 {currentUser?.deviceLimit ?? 10}{" "}
            台设备。添加时会检查设备是否可用。
          </DialogDescription>
        </DialogHeader>
        {binding?.scanEnabled && (
          <p className="text-xs text-muted-foreground">
            {bindingRequired
              ? "1. 绑定微信 → 2. 添加充电桩"
              : "1. 微信已连接 → 2. 添加充电桩"}
          </p>
        )}
        {!online && (
          <p role="alert" className="text-sm text-destructive">
            当前离线，请恢复连接后重试。
          </p>
        )}
        {step === "checking" && <p role="status">正在检查平台连接…</p>}
        {step === "error" && (
          <div className="space-y-3">
            <p role="alert">{error}</p>
            <Button
              disabled={!online}
              onClick={() => {
                setStep("checking")
                setRetry((value) => value + 1)
              }}
            >
              重新检查
            </Button>
          </div>
        )}
        {open && step === "binding" && (
          <>
            <YybBindingFlow
              continueLabel="继续添加充电桩"
              onBack={
                form.number || form.id || form.name
                  ? () => setStep("form")
                  : undefined
              }
              onContinue={(next) => {
                setBinding(next)
                setBindingRequired(false)
                setError("")
                setStep("form")
              }}
            />
          </>
        )}
        {step === "form" && (
          <>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            {bindingRequired && (
              <div className="space-y-2 border-b pb-4">
                <p>添加前需要先完成微信绑定。</p>
                <Button disabled={!online} onClick={() => setStep("binding")}>
                  去绑定 / 重新扫码
                </Button>
              </div>
            )}
            {binding?.scanEnabled !== true && (
              <div className="space-y-3">
                <Button
                  variant="ghost"
                  onClick={() => setManual((value) => !value)}
                >
                  手动设置 Cookie
                </Button>
                {manual && <ManualCookieForm />}
              </div>
            )}
            <form onSubmit={submit}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="pile-number">桩号</FieldLabel>
                  <Input
                    ref={numberInput}
                    id="pile-number"
                    inputMode="numeric"
                    autoComplete="off"
                    value={form.number}
                    onChange={(event) => change("number", event.target.value)}
                    placeholder="例如 61034278"
                  />
                  <FieldDescription>
                    输入充电桩上二维码上方或小程序中显示的桩号。
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="pile-name">显示名称</FieldLabel>
                  <Input
                    id="pile-name"
                    value={form.name}
                    onChange={(event) => change("name", event.target.value)}
                    placeholder="例如：松园 3 号楼北侧"
                  />
                </Field>
                <Button
                  type="button"
                  variant="ghost"
                  className="justify-start px-0"
                  onClick={() => setAdvanced((value) => !value)}
                >
                  {advanced ? "收起高级字段" : "填写设备长 ID、地址等高级字段"}
                </Button>
                {advanced && (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="pile-id">设备长 ID</FieldLabel>
                      <Input
                        id="pile-id"
                        inputMode="numeric"
                        value={form.id}
                        onChange={(event) => change("id", event.target.value)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="pile-ports">充电口数量</FieldLabel>
                      <Input
                        id="pile-ports"
                        type="number"
                        min="1"
                        max="20"
                        value={form.openNum}
                        onChange={(event) =>
                          change("openNum", Number(event.target.value))
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="pile-status">设备状态</FieldLabel>
                      <Input
                        id="pile-status"
                        value={form.status}
                        onChange={(event) =>
                          change("status", event.target.value)
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="pile-address">安装地址</FieldLabel>
                      <Input
                        id="pile-address"
                        value={form.address}
                        onChange={(event) =>
                          change("address", event.target.value)
                        }
                      />
                    </Field>
                  </div>
                )}
                <DialogFooter>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => handleOpenChange(false)}
                  >
                    取消
                  </Button>
                  <Button
                    type="submit"
                    disabled={submitting || bindingRequired || !online}
                  >
                    {submitting && (
                      <LoaderCircleIcon className="motion-safe:animate-spin" />
                    )}
                    {submitting ? "添加中…" : "确认添加"}
                  </Button>
                </DialogFooter>
              </FieldGroup>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
