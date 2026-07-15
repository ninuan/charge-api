"use client"

import { BookOpenCheckIcon, CheckCircle2Icon, MousePointer2Icon, ShieldCheckIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { useAuth } from "@/lib/auth-context"

const steps = [
  ["准备微信", "确保可以扫码登录", ["准备一台可以正常使用微信的手机。", "确认微信可以扫码并完成授权。", "本系统不会要求输入微信密码。"]],
  ["打开扫码登录", "在系统里生成二维码", ["回到用户看板页面。", "点击右上角的“扫码登录”。", "在弹窗中点击“生成二维码”。", "等待二维码显示出来，不要关闭弹窗。"]],
  ["使用微信扫码", "完成授权登录", ["使用微信扫描页面里的二维码。", "按微信页面提示完成确认。", "扫码后回到系统页面，点击“检查扫码状态”。"]],
  ["确认绑定状态", "让系统保存登录凭据", ["扫码完成后，点击“确认绑定”。", "如果提示“扫码登录已生效”，说明当前账号已经绑定成功。", "如果当前账号已经添加过充电桩，系统会尝试自动更新登录凭据。"]],
  ["添加充电桩", "输入桩号或设备长 ID", ["回到用户看板页面。", "点击“添加充电桩”。", "输入桩号或设备长 ID。", "点击添加后，系统会自动查询并保存该充电桩。"]],
  ["刷新查看状态", "查看充电口占用情况", ["添加成功后，充电桩会出现在看板中。", "点击“刷新状态”获取最新充电口占用情况。", "系统会显示每个充电口是空闲、使用中、离线还是异常。", "短时间重复刷新会优先返回缓存。"]],
] as const

export function UsageGuideDialog() {
  const { currentUser, acknowledgeUsageGuide } = useAuth()
  const [open, setOpen] = useState(false)
  const [required, setRequired] = useState(false)
  const [reachedEnd, setReachedEnd] = useState(false)
  const [saving, setSaving] = useState(false)
  const promptedRef = useRef("")
  useEffect(() => { const user = currentUser; if (!user || user.role !== "user" || user.usageGuideAckAt || promptedRef.current === user.id) return; promptedRef.current = user.id; setRequired(true); setReachedEnd(false); setOpen(true) }, [currentUser])
  function openReference() { setRequired(false); setReachedEnd(true); setOpen(true) }
  async function close() { if (required && !reachedEnd) return; if (!required) return setOpen(false); setSaving(true); try { await acknowledgeUsageGuide(); setRequired(false); setOpen(false) } catch (reason) { toast.error((reason as Error).message) } finally { setSaving(false) } }
  function handleOpen(next: boolean) { if (!next && required && !reachedEnd) return; setOpen(next) }
  function checkEnd(target: HTMLElement) { setReachedEnd(target.scrollTop + target.clientHeight >= target.scrollHeight - 8) }
  return <Dialog open={open} onOpenChange={handleOpen}><DialogTrigger render={<Button variant="outline" onClick={openReference}><BookOpenCheckIcon />使用说明</Button>} /><DialogContent showCloseButton={!required || reachedEnd} className="grid h-[min(50rem,calc(100dvh-2rem))] max-w-5xl grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0"><DialogHeader className="border-b p-5 sm:p-6"><div className="flex gap-3"><span className="rounded-lg bg-muted p-2"><BookOpenCheckIcon className="size-5" /></span><div><DialogTitle>扫码登录与充电桩添加说明</DialogTitle>{required && <DialogDescription className="mt-2">首次进入前请完整看完说明。完成扫码登录后，就可以添加充电桩并查看充电口状态。</DialogDescription>}</div></div></DialogHeader><div className="min-h-0 overflow-y-auto p-5 sm:p-6" onScroll={(event) => checkEnd(event.currentTarget)}><div className="grid gap-6 md:grid-cols-[12rem_1fr]"><aside className="hidden border-r pr-4 md:block"><p className="text-xs font-medium text-muted-foreground">操作路径</p><ol className="mt-3 space-y-3">{steps.map(([title], index) => <li key={title}><a className="text-sm hover:underline" href={`#guide-${index + 1}`}>{index + 1}. {title}</a></li>)}</ol></aside><div className="space-y-4">{steps.map(([title, detail, items], index) => <section id={`guide-${index + 1}`} key={title} className="rounded-lg border p-5"><div className="flex gap-3"><span className="grid size-7 shrink-0 place-items-center rounded-full bg-muted text-sm font-medium">{index + 1}</span><div><h3 className="font-semibold">{title}</h3><p className="mt-1 text-sm text-muted-foreground">{detail}</p></div></div><ol className="mt-4 list-decimal space-y-2 pl-5 text-sm leading-6 text-muted-foreground">{items.map((item) => <li key={item}>{item}</li>)}</ol></section>)}</div></div></div><DialogFooter className="mx-0 mb-0 rounded-none border-t bg-background p-4 sm:px-6"><div className="flex w-full flex-col justify-between gap-3 sm:flex-row sm:items-center"><p className="flex items-center gap-2 text-xs text-muted-foreground">{reachedEnd ? <ShieldCheckIcon className="size-4" /> : <MousePointer2Icon className="size-4" />}{reachedEnd ? "已读到说明底部，可以开始使用。" : "请继续向下滚动，看完整个说明。"}</p><Button disabled={!reachedEnd || saving} onClick={() => void close()}><CheckCircle2Icon />{saving ? "正在确认…" : required ? "我已看完并关闭" : "关闭"}</Button></div></DialogFooter></DialogContent></Dialog>
}
