"use client"

import {
  ActivityIcon,
  BarChart3Icon,
  CalendarDaysIcon,
  Clock3Icon,
  InfoIcon,
  RefreshCwIcon,
  SparklesIcon,
  WifiOffIcon,
} from "lucide-react"
import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type {
  DeviceHistoryResponse,
  HistoryHeatmapCell,
  HistoryHourInsight,
  HistorySampleState,
  PortHistoryMetrics,
  PortHistoryResponse,
  PortStatus,
} from "@/lib/api/generated"
import { historyApi } from "@/lib/history-api"
import { cn } from "@/lib/utils"
import type { Pile } from "@/lib/types"

const weekdays = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]

const statusLabels: Record<PortStatus, string> = {
  idle: "空闲",
  in_use: "充电中",
  offline: "离线",
}

const sampleLabels: Record<HistorySampleState, string> = {
  no_data: "暂无数据",
  insufficient: "样本不足",
  partial: "部分数据",
  sufficient: "样本充足",
}

type Props = {
  pile: Pile
  open: boolean
  onOpenChange: (open: boolean) => void
}

function resolveTimezone() {
  try {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
    return timezone === "UTC" || timezone.includes("/")
      ? timezone
      : "Asia/Shanghai"
  } catch {
    return "Asia/Shanghai"
  }
}

function isAbort(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError"
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "请求失败，请稍后重试"
}

function formatPercent(value: number | null) {
  return value === null ? "数据不足" : `${value.toFixed(1)}%`
}

function formatDuration(seconds: number | null) {
  if (seconds === null) return "数据不足"
  if (seconds < 60) return `${seconds} 秒`
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.round((seconds % 3600) / 60)
  if (!hours) return `${minutes} 分钟`
  return minutes ? `${hours} 小时 ${minutes} 分钟` : `${hours} 小时`
}

function formatDate(value: string) {
  const [, month = "", day = ""] = value.split("-")
  return `${Number(month)}月${Number(day)}日`
}

function formatTime(value: string, timezone: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: timezone,
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value))
}

function formatHourInsight(insight?: HistoryHourInsight) {
  if (!insight) return "样本积累中"
  return `${weekdays[insight.weekday - 1]} ${String(insight.hour).padStart(2, "0")}:00`
}

function SummaryMetric({
  label,
  value,
  detail,
}: {
  label: string
  value: string
  detail: string
}) {
  return (
    <Card size="sm" className="bg-muted/20 shadow-none">
      <CardHeader>
        <CardTitle className="text-xs font-medium text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-lg font-semibold tracking-tight tabular-nums">
          {value}
        </p>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</p>
      </CardContent>
    </Card>
  )
}

function SampleBadge({ state }: { state: HistorySampleState }) {
  return (
    <Badge variant={state === "sufficient" ? "secondary" : "outline"}>
      {sampleLabels[state]}
    </Badge>
  )
}

function DailyTrend({ data }: { data: DeviceHistoryResponse["daily"] }) {
  const [activeLabel, setActiveLabel] = useState<string | null>(null)

  return (
    <Card className="shadow-none">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BarChart3Icon className="size-4 text-primary" aria-hidden="true" />
          每日占用趋势
        </CardTitle>
        <CardDescription>
          离线时间不计入占用率，未知时段不会被当作空闲。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div
          className="flex h-44 items-end gap-2 rounded-lg bg-muted/20 px-2 pt-5 pb-2 sm:gap-3 sm:px-4"
          role="group"
          aria-label="最近七天每日占用率"
        >
          {data.map((point) => {
            const occupancy = point.metrics.occupancyPercent
            const label = `${formatDate(point.date)}，${formatPercent(occupancy)}，${sampleLabels[point.metrics.sampleState]}`
            return (
              <button
                key={point.date}
                type="button"
                className="group flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-2 rounded-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                aria-label={label}
                onFocus={() => setActiveLabel(label)}
                onMouseEnter={() => setActiveLabel(label)}
              >
                <span className="flex h-28 w-full max-w-9 items-end overflow-hidden rounded-md bg-muted">
                  <span
                    aria-hidden="true"
                    className={cn(
                      "w-full rounded-md transition-[height,background-color] duration-300",
                      occupancy === null
                        ? "h-1 bg-muted-foreground/20"
                        : "bg-primary"
                    )}
                    style={
                      occupancy === null
                        ? undefined
                        : { height: `${Math.max(occupancy, 4)}%` }
                    }
                  />
                </span>
                <span className="text-[10px] text-muted-foreground sm:text-xs">
                  {formatDate(point.date).replace("月", "/").replace("日", "")}
                </span>
              </button>
            )
          })}
        </div>
        <p
          className="mt-3 min-h-5 text-xs text-muted-foreground"
          aria-live="polite"
        >
          {activeLabel ?? "悬停或使用键盘聚焦柱形，可查看当天详细数据。"}
        </p>
      </CardContent>
    </Card>
  )
}

function heatmapLabel(cell: HistoryHeatmapCell) {
  return `${weekdays[cell.weekday - 1]} ${String(cell.hour).padStart(2, "0")}:00，${formatPercent(cell.occupancyPercent)}，覆盖 ${cell.sampleDates} 天${cell.sampleSufficient ? "，样本充足" : "，样本不足"}`
}

function HistoryHeatmap({ cells }: { cells: HistoryHeatmapCell[] }) {
  const [activeLabel, setActiveLabel] = useState<string | null>(null)
  const cellMap = useMemo(
    () => new Map(cells.map((cell) => [`${cell.weekday}-${cell.hour}`, cell])),
    [cells]
  )

  return (
    <Card className="shadow-none">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CalendarDaysIcon
            className="size-4 text-primary"
            aria-hidden="true"
          />
          常见繁忙时段
        </CardTitle>
        <CardDescription>
          颜色越深代表占用率越高；至少覆盖 3 天且累计 6 小时才参与提示。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto pb-2">
          <div
            className="grid min-w-[43rem] grid-cols-[2.5rem_repeat(24,1rem)] gap-1"
            role="group"
            aria-label="星期与小时占用率热力图"
          >
            <span />
            {Array.from({ length: 24 }, (_, hour) => (
              <span
                key={hour}
                className="text-center text-[9px] text-muted-foreground"
              >
                {hour % 3 === 0 ? hour : ""}
              </span>
            ))}
            {weekdays.map((weekday, dayIndex) => (
              <Fragment key={weekday}>
                <span className="self-center text-[10px] text-muted-foreground">
                  {weekday}
                </span>
                {Array.from({ length: 24 }, (_, hour) => {
                  const cell = cellMap.get(`${dayIndex + 1}-${hour}`)
                  const label = cell
                    ? heatmapLabel(cell)
                    : `${weekday} ${String(hour).padStart(2, "0")}:00，暂无数据`
                  return (
                    <button
                      key={hour}
                      type="button"
                      title={label}
                      aria-label={label}
                      className="relative size-4 rounded-[3px] bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:outline-none"
                      onFocus={() => setActiveLabel(label)}
                      onMouseEnter={() => setActiveLabel(label)}
                    >
                      {cell?.occupancyPercent !== null &&
                        cell?.occupancyPercent !== undefined && (
                          <span
                            aria-hidden="true"
                            className="absolute inset-0 rounded-[3px] bg-primary transition-opacity duration-200"
                            style={{
                              opacity: Math.max(
                                0.14,
                                cell.occupancyPercent / 100
                              ),
                            }}
                          />
                        )}
                    </button>
                  )
                })}
              </Fragment>
            ))}
          </div>
        </div>
        <div className="mt-2 flex items-center justify-between gap-3 text-xs text-muted-foreground">
          <span aria-live="polite">
            {activeLabel ?? "悬停或用键盘聚焦色块查看详细数据。"}
          </span>
          <span className="flex shrink-0 items-center gap-1" aria-hidden="true">
            低
            <span className="size-3 rounded-[3px] bg-primary/20" />
            <span className="size-3 rounded-[3px] bg-primary/55" />
            <span className="size-3 rounded-[3px] bg-primary" />高
          </span>
        </div>
      </CardContent>
    </Card>
  )
}

function portStatusBadge(status: PortStatus) {
  if (status === "offline") return "destructive" as const
  if (status === "in_use") return "secondary" as const
  return "outline" as const
}

function PortMetrics({ metrics }: { metrics: PortHistoryMetrics }) {
  return (
    <div className="grid grid-cols-3 gap-2">
      <div className="rounded-lg bg-muted/30 p-3">
        <p className="text-xs text-muted-foreground">占用率</p>
        <p className="mt-1 font-semibold tabular-nums">
          {formatPercent(metrics.occupancyPercent)}
        </p>
      </div>
      <div className="rounded-lg bg-muted/30 p-3">
        <p className="text-xs text-muted-foreground">完整会话</p>
        <p className="mt-1 font-semibold tabular-nums">
          {metrics.completedSessions} 次
        </p>
      </div>
      <div className="rounded-lg bg-muted/30 p-3">
        <p className="text-xs text-muted-foreground">平均时长</p>
        <p className="mt-1 font-semibold tabular-nums">
          {formatDuration(metrics.averageSessionSeconds)}
        </p>
      </div>
    </div>
  )
}

function PortHistoryDetail({
  data,
  timezone,
}: {
  data: PortHistoryResponse
  timezone: string
}) {
  const timeline = useMemo(() => data.timeline.toReversed(), [data.timeline])

  return (
    <div className="history-panel-enter flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="font-medium">{data.portId} 号充电口</h4>
          <p className="mt-0.5 text-xs text-muted-foreground">
            最近状态变化与完整充电会话统计
          </p>
        </div>
        <div className="flex items-center gap-2">
          <SampleBadge state={data.metrics.sampleState} />
          <Badge variant={portStatusBadge(data.currentStatus)}>
            {statusLabels[data.currentStatus]}
          </Badge>
        </div>
      </div>
      <PortMetrics metrics={data.metrics} />
      {timeline.length ? (
        <ol
          className="flex flex-col gap-2"
          aria-label={`${data.portId} 号口状态时间线`}
        >
          {timeline.map((item, index) => (
            <li
              key={`${item.changedAt}-${index}`}
              className="rounded-lg border bg-muted/15 p-3 [contain-intrinsic-size:auto_4.5rem] [content-visibility:auto]"
            >
              <div className="flex items-start justify-between gap-3">
                <p className="text-sm font-medium">
                  {item.fromStatus
                    ? `${statusLabels[item.fromStatus]} → ${statusLabels[item.toStatus]}`
                    : `开始记录为${statusLabels[item.toStatus]}`}
                </p>
                <time className="shrink-0 text-xs text-muted-foreground">
                  {formatTime(item.changedAt, timezone)}
                </time>
              </div>
              {(item.usedSeconds > 0 || item.remainingText) && (
                <p className="mt-1 text-xs text-muted-foreground">
                  已用 {formatDuration(item.usedSeconds)}
                  {item.remainingText ? ` · 剩余 ${item.remainingText}` : ""}
                </p>
              )}
            </li>
          ))}
        </ol>
      ) : (
        <Alert>
          <InfoIcon />
          <AlertTitle>暂时没有状态变化</AlertTitle>
          <AlertDescription>
            当前只有累计统计，后续端口状态改变后会在这里形成时间线。
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}

function HistoryLoading() {
  return (
    <div className="grid gap-3" aria-label="正在加载历史数据">
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-28 w-full" />
        ))}
      </div>
      <Skeleton className="h-64 w-full" />
      <Skeleton className="h-72 w-full" />
    </div>
  )
}

export function DeviceHistorySheet({ pile, open, onOpenChange }: Props) {
  const [timezone] = useState(resolveTimezone)
  const timezoneLabel = timezone === "Asia/Shanghai" ? "北京时间" : timezone
  const [device, setDevice] = useState<DeviceHistoryResponse | null>(null)
  const [deviceLoading, setDeviceLoading] = useState(false)
  const [deviceError, setDeviceError] = useState("")
  const [selectedPort, setSelectedPort] = useState<number | null>(
    () => pile.ports[0]?.id ?? null
  )
  const [port, setPort] = useState<PortHistoryResponse | null>(null)
  const [portLoading, setPortLoading] = useState(false)
  const [portError, setPortError] = useState("")
  const deviceController = useRef<AbortController | null>(null)
  const portController = useRef<AbortController | null>(null)

  const loadDevice = useCallback(async () => {
    deviceController.current?.abort()
    const controller = new AbortController()
    deviceController.current = controller
    setDeviceLoading(true)
    setDeviceError("")
    try {
      const next = await historyApi.device(
        pile.id,
        { range: "7d", timezone },
        { signal: controller.signal }
      )
      if (!controller.signal.aborted) setDevice(next)
    } catch (error) {
      if (!controller.signal.aborted && !isAbort(error))
        setDeviceError(errorMessage(error))
    } finally {
      if (!controller.signal.aborted) setDeviceLoading(false)
    }
  }, [pile.id, timezone])

  const loadPort = useCallback(
    async (portId: number) => {
      portController.current?.abort()
      const controller = new AbortController()
      portController.current = controller
      setPort(null)
      setPortLoading(true)
      setPortError("")
      try {
        const next = await historyApi.port(
          pile.id,
          portId,
          { range: "7d", timezone },
          { signal: controller.signal }
        )
        if (!controller.signal.aborted) setPort(next)
      } catch (error) {
        if (!controller.signal.aborted && !isAbort(error))
          setPortError(errorMessage(error))
      } finally {
        if (!controller.signal.aborted) setPortLoading(false)
      }
    },
    [pile.id, timezone]
  )

  const initialPortId = pile.ports[0]?.id ?? null
  useEffect(() => {
    if (!open) return
    let active = true
    queueMicrotask(() => {
      if (!active) return
      void loadDevice()
      if (initialPortId !== null) void loadPort(initialPortId)
    })
    return () => {
      active = false
      deviceController.current?.abort()
      portController.current?.abort()
    }
  }, [initialPortId, loadDevice, loadPort, open])

  function selectPort(value: string | number) {
    const portId = Number(value)
    if (!Number.isInteger(portId) || portId === selectedPort) return
    setSelectedPort(portId)
    void loadPort(portId)
  }

  const busiest = device?.busiestHours[0]
  const portIds =
    device?.ports.map((summary) => summary.portId) ??
    pile.ports.map((item) => item.id)

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-hidden p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-3xl data-[side=right]:lg:max-w-4xl">
        <SheetHeader className="border-b px-4 py-4 pr-14 sm:px-6 sm:pr-14">
          <div className="flex items-start gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <BarChart3Icon className="size-5" aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <SheetTitle className="flex flex-wrap items-center gap-2 text-lg">
                <span className="truncate">{pile.name}</span>
                <Badge variant="outline">最近 7 天</Badge>
              </SheetTitle>
              <SheetDescription className="mt-1">
                桩号 {pile.number || pile.id} · 按 {timezoneLabel}聚合
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <div
          data-slot="history-scroll"
          className="flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-5"
        >
          {deviceLoading && !device ? (
            <HistoryLoading />
          ) : deviceError && !device ? (
            <Alert variant="destructive">
              <WifiOffIcon />
              <AlertTitle>历史数据加载失败</AlertTitle>
              <AlertDescription>{deviceError}</AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-2 w-fit"
                onClick={() => void loadDevice()}
              >
                <RefreshCwIcon data-icon="inline-start" />
                重试
              </Button>
            </Alert>
          ) : device ? (
            <div className="flex flex-col gap-4">
              <Alert>
                <InfoIcon />
                <AlertTitle>统计口径</AlertTitle>
                <AlertDescription>{device.historyNotice}</AlertDescription>
              </Alert>

              {device.metrics.sampleState === "no_data" && (
                <Alert>
                  <ActivityIcon />
                  <AlertTitle>历史数据正在积累</AlertTitle>
                  <AlertDescription>
                    首次刷新只建立当前状态基线，之后端口状态发生变化才会形成趋势。
                  </AlertDescription>
                </Alert>
              )}

              <section aria-labelledby="history-summary-title">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <h3 id="history-summary-title" className="font-medium">
                    使用概览
                  </h3>
                  <SampleBadge state={device.metrics.sampleState} />
                </div>
                <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
                  <SummaryMetric
                    label="最近 7 天占用率"
                    value={formatPercent(device.metrics.occupancyPercent)}
                    detail="仅统计空闲与充电中时段"
                  />
                  <SummaryMetric
                    label="平均充电时长"
                    value={formatDuration(device.metrics.averageSessionSeconds)}
                    detail={`${device.metrics.completedSessions} 次完整会话`}
                  />
                  <SummaryMetric
                    label="常见繁忙时段"
                    value={formatHourInsight(busiest)}
                    detail={
                      busiest
                        ? `平均占用 ${formatPercent(busiest.occupancyPercent)}`
                        : "达到样本门槛后显示"
                    }
                  />
                  <SummaryMetric
                    label="通常较空闲"
                    value={formatHourInsight(device.quietSuggestion)}
                    detail={
                      device.quietSuggestion
                        ? `平均占用 ${formatPercent(device.quietSuggestion.occupancyPercent)}`
                        : "达到样本门槛后显示"
                    }
                  />
                </div>
              </section>

              <DailyTrend data={device.daily} />
              <HistoryHeatmap cells={device.heatmap} />

              <Card className="shadow-none">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Clock3Icon
                      className="size-4 text-primary"
                      aria-hidden="true"
                    />
                    单端口历史
                  </CardTitle>
                  <CardDescription>
                    使用方向键切换端口，查看最近状态变化和会话统计。
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {selectedPort !== null && portIds.length ? (
                    <Tabs
                      value={String(selectedPort)}
                      onValueChange={selectPort}
                    >
                      <div className="overflow-x-auto pb-1">
                        <TabsList
                          activateOnFocus
                          className="w-max min-w-full justify-start"
                        >
                          {portIds.map((portId) => (
                            <TabsTrigger key={portId} value={String(portId)}>
                              {String(portId).padStart(2, "0")} 号
                            </TabsTrigger>
                          ))}
                        </TabsList>
                      </div>
                      <TabsContent
                        value={String(selectedPort)}
                        className="pt-3"
                      >
                        {portLoading ? (
                          <div
                            className="grid gap-2"
                            aria-label="正在加载端口历史"
                          >
                            <Skeleton className="h-20 w-full" />
                            <Skeleton className="h-16 w-full" />
                            <Skeleton className="h-16 w-full" />
                          </div>
                        ) : portError ? (
                          <Alert variant="destructive">
                            <WifiOffIcon />
                            <AlertTitle>端口历史加载失败</AlertTitle>
                            <AlertDescription>{portError}</AlertDescription>
                            <Button
                              variant="outline"
                              size="sm"
                              className="mt-2 w-fit"
                              onClick={() => void loadPort(selectedPort)}
                            >
                              <RefreshCwIcon data-icon="inline-start" />
                              重试当前端口
                            </Button>
                          </Alert>
                        ) : port ? (
                          <PortHistoryDetail data={port} timezone={timezone} />
                        ) : null}
                      </TabsContent>
                    </Tabs>
                  ) : (
                    <Alert>
                      <SparklesIcon />
                      <AlertTitle>暂无可查看端口</AlertTitle>
                      <AlertDescription>
                        设备产生端口历史后，可在这里查看状态时间线。
                      </AlertDescription>
                    </Alert>
                  )}
                </CardContent>
              </Card>
            </div>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
