"use client"

import { BatteryChargingIcon, CheckCircle2Icon, ChevronDownIcon, Clock3Icon, MapPinIcon, PencilIcon, PlugZapIcon, Trash2Icon, WifiOffIcon, ZapIcon } from "lucide-react"
import { useEffect, useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { Pile, Port } from "@/lib/types"

type Props = { pile: Pile; visiblePortIds: number[]; filtering: boolean; onRemove: (id: string) => void; onUpdate: (id: string, payload: { name: string; address: string; sortOrder: number }) => void }

function portMeta(port: Port) {
  if (port.status === "in_use") return { label: "充电中", icon: ZapIcon, className: "border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-50" }
  if (port.status === "offline") return { label: "离线", icon: WifiOffIcon, className: "border-destructive/25 bg-destructive/5 text-destructive" }
  return { label: "空闲", icon: CheckCircle2Icon, className: "border-border bg-muted/60 text-foreground" }
}

export function PileCard({ pile, visiblePortIds, filtering, onRemove, onUpdate }: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [edit, setEdit] = useState({ name: pile.name, address: pile.address, sortOrder: pile.sortOrder ?? 0 })
  useEffect(() => setEdit({ name: pile.name, address: pile.address, sortOrder: pile.sortOrder ?? 0 }), [pile])
  const displayedPorts = useMemo(() => filtering ? pile.ports.filter((port) => visiblePortIds.includes(port.id)) : pile.ports, [filtering, pile.ports, visiblePortIds])
  const inUseCount = pile.ports.filter((port) => port.status === "in_use").length
  const idleCount = pile.ports.filter((port) => port.status === "idle").length
  const hasIssue = pile.ports.some((port) => port.status === "offline")
  const cardId = `pile-ports-${pile.id}`

  return <><Card className="overflow-hidden shadow-none"><CardHeader className="flex flex-col gap-4 border-b p-5 sm:flex-row sm:items-start sm:justify-between lg:p-6"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h2 className="truncate text-xl font-semibold tracking-tight">{pile.name}</h2><Badge variant={hasIssue ? "destructive" : inUseCount ? "secondary" : "outline"}>{hasIssue ? <WifiOffIcon /> : inUseCount ? <BatteryChargingIcon /> : <CheckCircle2Icon />}{hasIssue ? "存在离线端口" : inUseCount ? "正在充电" : "运行正常"}</Badge></div><div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground"><span className="inline-flex items-center gap-1.5"><PlugZapIcon className="size-3.5" />桩号 {pile.number || pile.id}</span>{pile.address && <span className="inline-flex items-center gap-1.5"><MapPinIcon className="size-3.5" />{pile.address}</span>}</div>{filtering && <p className="mt-3 text-xs font-medium text-primary">当前显示 {displayedPorts.length} / {pile.ports.length} 个匹配端口</p>}</div><div className="flex items-center justify-between gap-3 sm:justify-end"><div className="flex gap-4 text-right text-xs text-muted-foreground"><span><strong className="block text-lg font-semibold tabular-nums text-foreground">{inUseCount}</strong>使用中</span><span><strong className="block text-lg font-semibold tabular-nums text-foreground">{idleCount}</strong>空闲</span></div><div className="flex"><Button variant="ghost" size="icon" aria-label={collapsed ? "展开充电口" : "收起充电口"} aria-expanded={!collapsed} aria-controls={cardId} onClick={() => setCollapsed((value) => !value)}><ChevronDownIcon className={collapsed ? "-rotate-90 transition-transform" : "transition-transform"} /></Button><Button variant="ghost" size="icon" aria-label="编辑充电桩" onClick={() => setEditOpen(true)}><PencilIcon /></Button><Button variant="ghost" size="icon" aria-label="删除充电桩" onClick={() => setConfirmOpen(true)}><Trash2Icon /></Button></div></div></CardHeader>{!collapsed && <CardContent id={cardId} className="grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 lg:grid-cols-5 lg:p-6">{displayedPorts.map((port) => { const meta = portMeta(port); const Icon = meta.icon; return <section key={port.id} className={`rounded-lg border p-4 ${meta.className}`}><div className="flex items-center justify-between gap-2"><span className="text-lg font-semibold tabular-nums">{String(port.id).padStart(2, "0")}</span><Icon className="size-4" /></div><p className="mt-5 text-sm font-semibold">{meta.label}</p><div className="mt-2 min-h-9 space-y-1 text-xs leading-4 opacity-80">{port.status === "in_use" ? <><p className="flex items-center gap-1"><Clock3Icon className="size-3" />已用 {port.usedText ?? "--"}</p><p>剩余 {port.remainingText ?? "--"}</p></> : <p>等待连接</p>}</div></section>})}</CardContent>}</Card>
    <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}><DialogContent><DialogHeader><DialogTitle>删除“{pile.name}”？</DialogTitle><DialogDescription>设备会从你的看板中移除。该操作不会影响充电桩本身。</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" onClick={() => setConfirmOpen(false)}>取消</Button><Button variant="destructive" onClick={() => { setConfirmOpen(false); onRemove(pile.id) }}><Trash2Icon />确认删除</Button></DialogFooter></DialogContent></Dialog>
    <Dialog open={editOpen} onOpenChange={setEditOpen}><DialogContent><DialogHeader><DialogTitle>编辑设备资料</DialogTitle><DialogDescription>名称、地址和排序只影响你的个人看板。</DialogDescription></DialogHeader><form onSubmit={(event) => { event.preventDefault(); setEditOpen(false); onUpdate(pile.id, edit) }}><FieldGroup><Field><FieldLabel htmlFor={`name-${pile.id}`}>显示名称</FieldLabel><Input id={`name-${pile.id}`} value={edit.name} onChange={(event) => setEdit((current) => ({ ...current, name: event.target.value }))} /></Field><Field><FieldLabel htmlFor={`address-${pile.id}`}>安装地址</FieldLabel><Input id={`address-${pile.id}`} value={edit.address} onChange={(event) => setEdit((current) => ({ ...current, address: event.target.value }))} /></Field><Field><FieldLabel htmlFor={`order-${pile.id}`}>排序值</FieldLabel><Input id={`order-${pile.id}`} type="number" value={edit.sortOrder} onChange={(event) => setEdit((current) => ({ ...current, sortOrder: Number(event.target.value) }))} /></Field><DialogFooter><Button type="button" variant="outline" onClick={() => setEditOpen(false)}>取消</Button><Button type="submit">保存修改</Button></DialogFooter></FieldGroup></form></DialogContent></Dialog>
  </>
}
