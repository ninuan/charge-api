"use client"

import {
  ArrowDownIcon,
  ArrowUpIcon,
  BatteryChargingIcon,
  BarChart3Icon,
  CheckCircle2Icon,
  ChevronDownIcon,
  Clock3Icon,
  MapPinIcon,
  PencilIcon,
  PlugZapIcon,
  Trash2Icon,
  WifiOffIcon,
  ZapIcon,
} from "lucide-react"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { Pile, Port } from "@/lib/types"

type Props = {
  pile: Pile
  visiblePortIds: number[]
  filtering: boolean
  onRemove: (id: string) => void
  onUpdate: (
    id: string,
    payload: { name: string; address: string; sortOrder: number }
  ) => void
  canMoveUp: boolean
  canMoveDown: boolean
  reordering: boolean
  onMove: (id: string, direction: "up" | "down") => void | Promise<void>
  onHistory: (id: string) => void
}

function portMeta(port: Port) {
  if (port.status === "in_use")
    return {
      label: "充电中",
      icon: ZapIcon,
      className:
        "border-warning/30 bg-warning/10 text-warning-foreground dark:bg-warning/15",
    }
  if (port.status === "offline")
    return {
      label: "离线",
      icon: WifiOffIcon,
      className: "border-destructive/25 bg-destructive/5 text-destructive",
    }
  return {
    label: "空闲",
    icon: CheckCircle2Icon,
    className:
      "border-success/25 bg-success/10 text-success-foreground dark:bg-success/15",
  }
}

function PortStatusCard({ port }: { port: Port }) {
  const cardRef = useRef<HTMLElement>(null)
  const mounted = useRef(false)
  const previousStatus = useRef(port.status)
  const meta = portMeta(port)
  const Icon = meta.icon
  const setCardRef = useCallback((card: HTMLElement | null) => {
    cardRef.current = card
    if (!card || mounted.current) return
    mounted.current = true
    if (
      typeof window.matchMedia !== "function" ||
      !window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      card.classList.add("port-card-enter")
    }
  }, [])

  useEffect(() => {
    if (previousStatus.current === port.status) return
    previousStatus.current = port.status

    const card = cardRef.current
    if (
      !card ||
      (typeof window.matchMedia === "function" &&
        window.matchMedia("(prefers-reduced-motion: reduce)").matches)
    )
      return

    card.classList.remove("port-card-enter", "port-state-changed")
    void card.offsetWidth
    card.classList.add("port-state-changed")
  }, [port.status])

  return (
    <section
      ref={setCardRef}
      aria-label={`${port.id} 号充电口`}
      className={`rounded-lg border p-4 transition-[color,background-color,border-color,box-shadow] duration-200 hover:shadow-sm ${meta.className}`}
      onAnimationEnd={(event) => {
        if (event.target !== event.currentTarget) return
        event.currentTarget.classList.remove(
          "port-card-enter",
          "port-state-changed"
        )
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-lg font-semibold tabular-nums">
          {String(port.id).padStart(2, "0")}
        </span>
        <Icon className="size-4" />
      </div>
      <p className="mt-5 text-sm font-semibold">{meta.label}</p>
      <div className="mt-2 min-h-9 space-y-1 text-xs leading-4 opacity-80">
        {port.status === "in_use" ? (
          <>
            <p className="flex items-center gap-1">
              <Clock3Icon className="size-3" />
              已用 {port.usedText ?? "--"}
            </p>
            <p>剩余 {port.remainingText ?? "--"}</p>
          </>
        ) : port.status === "idle" ? (
          <p>等待使用</p>
        ) : (
          <p>设备暂不可访问</p>
        )}
      </div>
    </section>
  )
}

function PileCardComponent({
  pile,
  visiblePortIds,
  filtering,
  onRemove,
  onUpdate,
  canMoveUp,
  canMoveDown,
  reordering,
  onMove,
  onHistory,
}: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [edit, setEdit] = useState({
    name: pile.name,
    address: pile.address,
  })
  const displayedPorts = useMemo(
    () =>
      filtering
        ? pile.ports.filter((port) => visiblePortIds.includes(port.id))
        : pile.ports,
    [filtering, pile.ports, visiblePortIds]
  )
  const inUseCount = pile.ports.filter(
    (port) => port.status === "in_use"
  ).length
  const idleCount = pile.ports.filter((port) => port.status === "idle").length
  const hasIssue = pile.ports.some((port) => port.status === "offline")
  const cardId = `pile-ports-${pile.id}`
  function openEdit() {
    setEdit({
      name: pile.name,
      address: pile.address,
    })
    setEditOpen(true)
  }

  return (
    <>
      <Card className="overflow-hidden shadow-xs transition-shadow hover:shadow-sm">
        <CardHeader className="flex flex-col gap-4 border-b p-5 sm:flex-row sm:items-start sm:justify-between lg:p-6">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-xl font-semibold tracking-tight">
                {pile.name}
              </h2>
              <Badge
                variant={
                  hasIssue
                    ? "destructive"
                    : inUseCount
                      ? "secondary"
                      : "outline"
                }
                className={
                  hasIssue
                    ? undefined
                    : inUseCount
                      ? "border-warning/30 bg-warning/10 text-warning-foreground"
                      : "border-success/30 bg-success/10 text-success-foreground"
                }
              >
                {hasIssue ? (
                  <WifiOffIcon />
                ) : inUseCount ? (
                  <BatteryChargingIcon />
                ) : (
                  <CheckCircle2Icon />
                )}
                {hasIssue
                  ? "存在离线端口"
                  : inUseCount
                    ? "正在充电"
                    : "运行正常"}
              </Badge>
            </div>
            <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1.5">
                <PlugZapIcon className="size-3.5" />
                桩号 {pile.number || pile.id}
              </span>
              {pile.address && (
                <span className="inline-flex items-center gap-1.5">
                  <MapPinIcon className="size-3.5" />
                  {pile.address}
                </span>
              )}
            </div>
            {filtering && (
              <p className="mt-3 text-xs font-medium text-primary">
                当前显示 {displayedPorts.length} / {pile.ports.length}{" "}
                个匹配端口
              </p>
            )}
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => onHistory(pile.id)}
            >
              <BarChart3Icon data-icon="inline-start" />
              历史趋势
            </Button>
          </div>
          <div className="flex items-center justify-between gap-3 sm:justify-end">
            <div className="flex gap-4 text-right text-xs text-muted-foreground">
              <span>
                <strong className="block text-lg font-semibold text-foreground tabular-nums">
                  {inUseCount}
                </strong>
                使用中
              </span>
              <span>
                <strong className="block text-lg font-semibold text-foreground tabular-nums">
                  {idleCount}
                </strong>
                空闲
              </span>
            </div>
            <div className="flex">
              <Button
                variant="ghost"
                size="icon"
                aria-label="上移充电桩"
                disabled={!canMoveUp || reordering}
                onClick={() => onMove(pile.id, "up")}
              >
                <ArrowUpIcon />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="下移充电桩"
                disabled={!canMoveDown || reordering}
                onClick={() => onMove(pile.id, "down")}
              >
                <ArrowDownIcon />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label={collapsed ? "展开充电口" : "收起充电口"}
                aria-expanded={!collapsed}
                aria-controls={cardId}
                onClick={() => setCollapsed((value) => !value)}
              >
                <ChevronDownIcon
                  className={
                    collapsed
                      ? "-rotate-90 transition-transform"
                      : "transition-transform"
                  }
                />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="编辑充电桩"
                onClick={openEdit}
              >
                <PencilIcon />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="删除充电桩"
                onClick={() => setConfirmOpen(true)}
              >
                <Trash2Icon />
              </Button>
            </div>
          </div>
        </CardHeader>
        {!collapsed && (
          <CardContent
            id={cardId}
            className="grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 lg:grid-cols-5 lg:p-6"
          >
            {displayedPorts.map((port) => (
              <PortStatusCard key={port.id} port={port} />
            ))}
          </CardContent>
        )}
      </Card>
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除“{pile.name}”？</DialogTitle>
            <DialogDescription>
              设备会从你的看板中移除。该操作不会影响充电桩本身。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmOpen(false)
                onRemove(pile.id)
              }}
            >
              <Trash2Icon />
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑设备资料</DialogTitle>
            <DialogDescription>
              名称和地址只影响你的个人看板；顺序可直接在设备卡片上调整。
            </DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault()
              setEditOpen(false)
              onUpdate(pile.id, {
                ...edit,
                sortOrder: pile.sortOrder ?? 0,
              })
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={`name-${pile.id}`}>显示名称</FieldLabel>
                <Input
                  id={`name-${pile.id}`}
                  value={edit.name}
                  onChange={(event) =>
                    setEdit((current) => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`address-${pile.id}`}>安装地址</FieldLabel>
                <Input
                  id={`address-${pile.id}`}
                  value={edit.address}
                  onChange={(event) =>
                    setEdit((current) => ({
                      ...current,
                      address: event.target.value,
                    }))
                  }
                />
              </Field>
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setEditOpen(false)}
                >
                  取消
                </Button>
                <Button type="submit">保存修改</Button>
              </DialogFooter>
            </FieldGroup>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}

function samePortIds(a: number[], b: number[]) {
  return (
    a === b ||
    (a.length === b.length && a.every((id, index) => id === b[index]))
  )
}

function samePile(a: Pile, b: Pile) {
  return (
    a === b ||
    (a.id === b.id &&
      a.number === b.number &&
      a.name === b.name &&
      a.status === b.status &&
      a.address === b.address &&
      a.openNum === b.openNum &&
      a.online === b.online &&
      a.createdAt === b.createdAt &&
      a.updatedAt === b.updatedAt &&
      a.source === b.source &&
      a.sortOrder === b.sortOrder &&
      samePortIds(a.usedPortIds, b.usedPortIds) &&
      a.ports.length === b.ports.length &&
      a.ports.every((port, index) => {
        const other = b.ports[index]
        return (
          other !== undefined &&
          port.id === other.id &&
          port.status === other.status &&
          port.powerKw === other.powerKw &&
          port.energyKwh === other.energyKwh &&
          port.updatedAt === other.updatedAt &&
          port.startedAt === other.startedAt &&
          port.sessionMin === other.sessionMin &&
          port.usedSeconds === other.usedSeconds &&
          port.usedText === other.usedText &&
          port.remainingText === other.remainingText
        )
      }))
  )
}

// SSE 每帧都会重建全部 pile 对象、筛选每次都会新建 portIds 数组，
// 引用比较永远不相等；按内容比较才能让未变化的卡片跳过重渲染。
export const PileCard = memo(
  PileCardComponent,
  (prev, next) =>
    prev.filtering === next.filtering &&
    prev.onRemove === next.onRemove &&
    prev.onUpdate === next.onUpdate &&
    prev.onMove === next.onMove &&
    prev.onHistory === next.onHistory &&
    prev.canMoveUp === next.canMoveUp &&
    prev.canMoveDown === next.canMoveDown &&
    prev.reordering === next.reordering &&
    samePortIds(prev.visiblePortIds, next.visiblePortIds) &&
    samePile(prev.pile, next.pile)
)
