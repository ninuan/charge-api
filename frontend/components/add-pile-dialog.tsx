"use client"

import {
  DatabaseIcon,
  PlusIcon,
  ServerCogIcon,
  SparklesIcon,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { useOnline } from "@/lib/browser-state"
import { notify } from "@/lib/feedback"
import { useCloseAppShellMenu } from "@/components/app-shell"
import {
  WorkbenchButton as Button,
  WorkbenchSearch,
} from "@/components/workbench/surfaces"
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
import type { Pile, SharedPile } from "@/lib/types"

export function AddPileDialog({
  onAdded,
  disabled = false,
}: { onAdded?: (pile: Pile) => void; disabled?: boolean } = {}) {
  const { currentUser } = useAuth()
  const { snapshot, addPile, searchSharedPiles } = useDashboard()
  const closeAppShellMenu = useCloseAppShellMenu()
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<"catalog" | "contribute">("catalog")
  const online = useOnline()

  const [catalogQuery, setCatalogQuery] = useState("")
  const [catalogItems, setCatalogItems] = useState<SharedPile[]>([])
  const [catalogLoading, setCatalogLoading] = useState(false)
  const [addingSharedId, setAddingSharedId] = useState<string | null>(null)

  const [error, setError] = useState("")
  const [advanced, setAdvanced] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const submitLock = useRef(false)
  const numberInput = useRef<HTMLInputElement>(null)
  const [form, setForm] = useState({
    id: "",
    name: "",
    number: "",
    openNum: 10,
    status: "在线",
    address: "",
  })

  const ownedIds = new Set(snapshot.piles.map((p) => p.id))
  const bonusQuota = snapshot.quota?.pileContributionBonusQuota ?? 2

  useEffect(() => {
    if (!open || mode !== "catalog") return
    let active = true
    const timer = window.setTimeout(() => {
      setCatalogLoading(true)
      searchSharedPiles(catalogQuery)
        .then((items) => {
          if (active) setCatalogItems(items)
        })
        .catch(() => {
          if (active) setCatalogItems([])
        })
        .finally(() => {
          if (active) setCatalogLoading(false)
        })
    }, 150)
    return () => {
      active = false
      window.clearTimeout(timer)
    }
  }, [open, mode, catalogQuery, searchSharedPiles])

  useEffect(() => {
    if (open && mode === "contribute") numberInput.current?.focus()
  }, [open, mode])

  const change = (name: keyof typeof form, value: string | number) =>
    setForm((current) => ({ ...current, [name]: value }))

  const handleOpenChange = (next: boolean) => {
    if (next) {
      setError("")
    }
    setOpen(next)
    if (!next) closeAppShellMenu()
  }

  async function addFromCatalog(item: SharedPile) {
    if (addingSharedId || !online) return
    setAddingSharedId(item.deviceId)
    setError("")
    try {
      const added = await addPile({
        id: item.deviceId,
        name: item.name || "",
        number: item.number,
        openNum: item.openNum || 10,
        status: "在线",
        address: item.address || "",
      })
      handleOpenChange(false)
      notify.success("已从公共桩库添加到常用列表")
      onAdded?.(added)
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "添加失败，请稍后重试。"
      )
    } finally {
      setAddingSharedId(null)
    }
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (submitLock.current || !navigator.onLine) return
    submitLock.current = true
    setError("")
    const id = form.id.trim()
    const number = form.number.trim()
    setSubmitting(true)
    try {
      const added = await addPile({
        id,
        name: form.name.trim(),
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
      if (added.rewardGranted && added.rewardGranted > 0) {
        notify.success(
          `新桩收录成功！奖励 +${added.rewardGranted} 次永久刷新额度`,
          { description: "奖励次数不会跨天清零，用完即止。" }
        )
      } else if (added.fromSharedCatalog) {
        notify.success("该桩已在公共库中，已为你添加到常用列表")
      } else {
        notify.success("充电桩已添加")
      }
      onAdded?.(added)
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "添加充电桩失败，请稍后重试。"
      )
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
            添加充电桩
          </DialogTitle>
          <DialogDescription>
            可直接从全站公共桩库一键添加；如果公共库里还没有，录入新桩可获赠{" "}
            <strong>+{bonusQuota} 次</strong> 永久刷新额度（用完即止）。
          </DialogDescription>
        </DialogHeader>

        <div className="wb-seg-rail" role="tablist" aria-label="添加方式">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "catalog"}
            className={`wb-seg-btn ${mode === "catalog" ? "wb-seg-active" : ""}`}
            onClick={() => {
              setMode("catalog")
              setError("")
            }}
          >
            <DatabaseIcon size={14} />
            从公共桩库添加
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "contribute"}
            className={`wb-seg-btn ${mode === "contribute" ? "wb-seg-active" : ""}`}
            onClick={() => {
              setMode("contribute")
              setError("")
            }}
          >
            <SparklesIcon size={14} />
            录入新桩 (+{bonusQuota} 次)
          </button>
        </div>

        {mode === "catalog" ? (
          <div className="space-y-3">
            <WorkbenchSearch
              value={catalogQuery}
              onChange={setCatalogQuery}
              label="搜索公共桩库"
              placeholder="按桩名、桩号或位置搜索公共桩库…"
            />

            {catalogLoading ? (
              <p className="py-6 text-center text-sm opacity-70">
                正在搜索公共桩库…
              </p>
            ) : catalogItems.length === 0 ? (
              <div className="rounded-lg border p-4 text-center text-sm">
                <p className="opacity-75">公共桩库中暂无匹配的充电桩。</p>
                <Button
                  type="button"
                  variant="outline"
                  className="mt-3"
                  onClick={() => setMode("contribute")}
                >
                  <SparklesIcon size={14} />
                  去录入这个新桩（+{bonusQuota} 次永久额度）
                </Button>
              </div>
            ) : (
              <div className="max-h-72 space-y-2 overflow-y-auto pr-1">
                {catalogItems.map((item) => {
                  const owned = ownedIds.has(item.deviceId)
                  return (
                    <div
                      key={item.deviceId}
                      className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5"
                    >
                      <div className="min-w-0">
                        <div className="truncate font-medium">
                          {item.name || `充电桩 ${item.number || item.deviceId.slice(-6)}`}
                        </div>
                        <div className="truncate text-xs opacity-70">
                          桩号 {item.number || "—"} · {item.openNum || 0} 口
                          {item.address ? ` · ${item.address}` : ""}
                        </div>
                      </div>
                      <Button
                        type="button"
                        variant={owned ? "ghost" : "outline"}
                        disabled={owned || !online || addingSharedId === item.deviceId}
                        busy={addingSharedId === item.deviceId}
                        onClick={() => void addFromCatalog(item)}
                      >
                        {owned ? "已添加" : "添加"}
                      </Button>
                    </div>
                  )
                })}
              </div>
            )}
            {error && (
              <p role="alert" className="text-destructive text-sm">
                {error}
              </p>
            )}
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="pile-number">桩号（推荐）</FieldLabel>
                <Input
                  ref={numberInput}
                  id="pile-number"
                  inputMode="numeric"
                  value={form.number}
                  onChange={(e) => change("number", e.target.value)}
                  placeholder="例如：61034278"
                />
                <FieldDescription>
                  输入桩身二维码下方的数字桩号即可，系统会自动校验并收录到公共桩库。
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="pile-name">常用名称（可选）</FieldLabel>
                <Input
                  id="pile-name"
                  value={form.name}
                  onChange={(e) => change("name", e.target.value)}
                  placeholder="例如：北门车棚 3 号桩"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="pile-address">位置备注（可选）</FieldLabel>
                <Input
                  id="pile-address"
                  value={form.address}
                  onChange={(e) => change("address", e.target.value)}
                  placeholder="例如：1 号楼东侧"
                />
              </Field>
              <div>
                <button
                  type="button"
                  className="text-xs underline opacity-75"
                  onClick={() => setAdvanced((prev) => !prev)}
                >
                  {advanced ? "收起高级选项" : "没有短桩号？填写设备长 ID"}
                </button>
              </div>
              {advanced && (
                <Field>
                  <FieldLabel htmlFor="pile-id">设备长 ID（可选）</FieldLabel>
                  <Input
                    id="pile-id"
                    inputMode="numeric"
                    value={form.id}
                    onChange={(e) => change("id", e.target.value)}
                    placeholder="例如：2601201412385560001"
                  />
                </Field>
              )}
            </FieldGroup>

            {error && (
              <p role="alert" className="text-destructive text-sm">
                {error}
              </p>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={submitting}
                onClick={() => handleOpenChange(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                busy={submitting}
                disabled={
                  !online ||
                  !currentUser?.refreshEnabled ||
                  (!form.number.trim() && !form.id.trim())
                }
              >
                校验并收录
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
