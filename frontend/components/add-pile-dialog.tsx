"use client"

import { PlusIcon, ServerCogIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/lib/auth-context"
import { useDashboard } from "@/lib/dashboard-context"

export function AddPileDialog() {
  const { currentUser } = useAuth()
  const { addPile } = useDashboard()
  const [open, setOpen] = useState(false)
  const [advanced, setAdvanced] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form, setForm] = useState({ id: "", name: "", number: "", openNum: 10, status: "在线", address: "" })
  const change = (name: keyof typeof form, value: string | number) => setForm((current) => ({ ...current, [name]: value }))

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const id = form.id.trim()
    const number = form.number.trim()
    setSubmitting(true)
    try {
      await addPile({ id, name: form.name.trim() || `充电桩 ${number || id.slice(-6)}`, number, openNum: Number(form.openNum), status: form.status.trim() || "在线", address: form.address.trim() })
      setForm({ id: "", name: "", number: "", openNum: 10, status: "在线", address: "" })
      setOpen(false)
      toast.success("充电桩已添加")
    } catch (reason) { toast.error((reason as Error).message) } finally { setSubmitting(false) }
  }

  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger render={<Button><PlusIcon />添加充电桩</Button>} /><DialogContent className="max-w-xl"><DialogHeader><DialogTitle className="flex items-center gap-2"><ServerCogIcon className="size-5" />添加充电桩</DialogTitle><DialogDescription>当前账户最多添加 {currentUser?.deviceLimit ?? 10} 台设备。完成扫码登录后，添加设备会自动更新访问凭据并验证远端数据。</DialogDescription></DialogHeader><form onSubmit={submit}><FieldGroup><Field><FieldLabel htmlFor="pile-number">桩号</FieldLabel><Input id="pile-number" inputMode="numeric" autoComplete="off" value={form.number} onChange={(event) => change("number", event.target.value)} placeholder="例如 61034278" /><FieldDescription>输入充电桩上二维码上方或小程序中显示的桩号。</FieldDescription></Field><Field><FieldLabel htmlFor="pile-name">显示名称</FieldLabel><Input id="pile-name" value={form.name} onChange={(event) => change("name", event.target.value)} placeholder="例如：松园 3 号楼北侧" /></Field><Button type="button" variant="ghost" className="justify-start px-0" onClick={() => setAdvanced((value) => !value)}>{advanced ? "收起高级字段" : "填写设备长 ID、地址等高级字段"}</Button>{advanced && <div className="grid gap-4 sm:grid-cols-2"><Field><FieldLabel htmlFor="pile-id">设备长 ID</FieldLabel><Input id="pile-id" inputMode="numeric" value={form.id} onChange={(event) => change("id", event.target.value)} /></Field><Field><FieldLabel htmlFor="pile-ports">充电口数量</FieldLabel><Input id="pile-ports" type="number" min="1" max="20" value={form.openNum} onChange={(event) => change("openNum", Number(event.target.value))} /></Field><Field><FieldLabel htmlFor="pile-status">设备状态</FieldLabel><Input id="pile-status" value={form.status} onChange={(event) => change("status", event.target.value)} /></Field><Field><FieldLabel htmlFor="pile-address">安装地址</FieldLabel><Input id="pile-address" value={form.address} onChange={(event) => change("address", event.target.value)} /></Field></div>}<DialogFooter><Button type="button" variant="outline" onClick={() => setOpen(false)}>取消</Button><Button type="submit" disabled={submitting}>{submitting ? "添加中…" : "确认添加"}</Button></DialogFooter></FieldGroup></form></DialogContent></Dialog>
}
