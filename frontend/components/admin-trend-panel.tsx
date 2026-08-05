"use client"

import {
  AlertCircleIcon,
  BarChart3Icon,
  DownloadIcon,
  LoaderCircleIcon,
  RefreshCwIcon,
} from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { adminApi } from "@/lib/admin-api"
import type {
  AdminTrendPoint,
  AdminTrendRange,
  AdminTrendsResponse,
} from "@/lib/api/generated"
import { cn } from "@/lib/utils"

type TrendMetric =
  "remoteSuccessRate" | "requests" | "activeUsers" | "offlinePorts"

type AdminTrendPanelProps = {
  trends: AdminTrendsResponse | null
  requestedRange: AdminTrendRange
  loading: boolean
  error: string | null
  onRangeChange: (range: AdminTrendRange) => void
  onReload: () => void
}

const ranges: { value: AdminTrendRange; label: string }[] = [
  { value: "24h", label: "24 小时" },
  { value: "7d", label: "7 天" },
  { value: "30d", label: "30 天" },
]

const metrics: { value: TrendMetric; label: string; unit: string }[] = [
  { value: "remoteSuccessRate", label: "远端成功率", unit: "%" },
  { value: "requests", label: "请求量", unit: "次" },
  { value: "activeUsers", label: "活跃用户", unit: "人" },
  { value: "offlinePorts", label: "离线端口", unit: "个" },
]

const numberFormatter = new Intl.NumberFormat("zh-CN")

function pointValue(point: AdminTrendPoint, metric: TrendMetric) {
  return point[metric]
}

function formatValue(value: number | null, metric: TrendMetric) {
  if (value === null) return "暂无数据"
  if (metric === "remoteSuccessRate") return `${value.toFixed(1)}%`
  return `${numberFormatter.format(value)} ${metrics.find((item) => item.value === metric)?.unit}`
}

function summaryDetail(trends: AdminTrendsResponse, metric: TrendMetric) {
  switch (metric) {
    case "remoteSuccessRate":
      return `${numberFormatter.format(trends.summary.remoteSuccesses)} / ${numberFormatter.format(trends.summary.remoteAttempts)} 次远端请求成功`
    case "requests":
      return "当前范围内的全部页面与操作请求"
    case "activeUsers":
      return "当前范围内按账户去重"
    case "offlinePorts":
      return "范围结束时的最新状态快照"
  }
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = filename
  document.body.append(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1_000)
}

export function AdminTrendPanel({
  trends,
  requestedRange,
  loading,
  error,
  onRangeChange,
  onReload,
}: AdminTrendPanelProps) {
  const [metric, setMetric] = useState<TrendMetric>("remoteSuccessRate")
  const [activeIndex, setActiveIndex] = useState<number | null>(null)
  const focusedIndex = useRef<number | null>(null)
  const [exporting, setExporting] = useState(false)
  const timezone = trends?.window.timezone ?? "Asia/Shanghai"
  const formatters = useMemo(
    () => ({
      tick: new Intl.DateTimeFormat("zh-CN", {
        timeZone: timezone,
        month: trends?.window.bucketUnit === "day" ? "2-digit" : undefined,
        day: trends?.window.bucketUnit === "day" ? "2-digit" : undefined,
        hour: trends?.window.bucketUnit === "hour" ? "2-digit" : undefined,
        minute: trends?.window.bucketUnit === "hour" ? "2-digit" : undefined,
        hour12: false,
      }),
      detail: new Intl.DateTimeFormat("zh-CN", {
        timeZone: timezone,
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      }),
      updated: new Intl.DateTimeFormat("zh-CN", {
        timeZone: timezone,
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      }),
    }),
    [timezone, trends?.window.bucketUnit]
  )

  const selectedMetric = metrics.find((item) => item.value === metric)!
  const values = trends?.points.map((point) => pointValue(point, metric)) ?? []
  const maximum =
    metric === "remoteSuccessRate"
      ? 100
      : Math.max(1, ...values.map((value) => value ?? 0))
  const summaryValue = trends ? trends.summary[metric] : null
  const labelStep = Math.max(1, Math.ceil(values.length / 6))
  const activePoint =
    activeIndex === null ? null : (trends?.points[activeIndex] ?? null)

  async function exportCSV() {
    setExporting(true)
    try {
      const file = await adminApi.trendsCSV(requestedRange, timezone)
      triggerDownload(file.blob, file.filename)
      toast.success(
        `${ranges.find((item) => item.value === requestedRange)?.label}趋势已导出`
      )
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setExporting(false)
    }
  }

  function pointLabel(point: AdminTrendPoint) {
    const period = `${formatters.detail.format(new Date(point.start))} 至 ${formatters.detail.format(new Date(point.end))}`
    const value = pointValue(point, metric)
    if (metric === "remoteSuccessRate" && value === null) {
      return `${period}，${selectedMetric.label}暂无数据，无远端请求`
    }
    return `${period}，${selectedMetric.label} ${formatValue(value, metric)}`
  }

  return (
    <Card className="shadow-xs" aria-busy={loading}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BarChart3Icon className="size-4 text-primary" aria-hidden="true" />
          运营趋势
        </CardTitle>
        <CardDescription>
          按统一时间范围分析远端质量、访问活跃度与端口健康状态。
        </CardDescription>
        <CardAction>
          <Button
            variant="outline"
            size="sm"
            disabled={
              !trends ||
              trends.window.range !== requestedRange ||
              loading ||
              exporting
            }
            onClick={() => void exportCSV()}
          >
            {exporting ? (
              <LoaderCircleIcon
                data-icon="inline-start"
                className="animate-spin"
              />
            ) : (
              <DownloadIcon data-icon="inline-start" />
            )}
            {exporting ? "正在导出" : "导出 CSV"}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <Tabs
          value={requestedRange}
          onValueChange={(value) => {
            focusedIndex.current = null
            setActiveIndex(null)
            onRangeChange(value as AdminTrendRange)
          }}
        >
          <TabsList
            className="grid h-auto w-full grid-cols-3"
            aria-label="趋势时间范围"
          >
            {ranges.map((range) => (
              <TabsTrigger key={range.value} value={range.value}>
                {range.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <Tabs
          value={metric}
          onValueChange={(value) => {
            focusedIndex.current = null
            setActiveIndex(null)
            setMetric(value as TrendMetric)
          }}
        >
          <TabsList
            className="grid h-auto w-full grid-cols-2 md:grid-cols-4"
            aria-label="趋势指标"
          >
            {metrics.map((item) => (
              <TabsTrigger key={item.value} value={item.value}>
                {item.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        {error ? (
          <Alert variant="destructive">
            <AlertCircleIcon aria-hidden="true" />
            <AlertTitle>趋势加载失败</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
            <Button variant="outline" size="sm" onClick={onReload}>
              <RefreshCwIcon data-icon="inline-start" />
              重新加载
            </Button>
          </Alert>
        ) : null}
        {!trends ? (
          <div className="flex flex-col gap-3" aria-label="正在加载运营趋势">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-56 w-full" />
          </div>
        ) : (
          <div
            key={trends.window.range}
            className={cn(
              "admin-trend-enter flex flex-col gap-4 transition-opacity duration-200",
              loading && "opacity-45"
            )}
          >
            <div className="flex flex-wrap items-end justify-between gap-3 rounded-lg bg-muted/25 px-4 py-3">
              <div>
                <p className="text-xs text-muted-foreground">
                  {selectedMetric.label}
                </p>
                <p className="mt-1 text-2xl font-semibold tracking-tight tabular-nums">
                  {formatValue(summaryValue, metric)}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {summaryDetail(trends, metric)}
                </p>
              </div>
              <div className="text-right text-xs leading-5 text-muted-foreground">
                <p>
                  {trends.window.bucketUnit === "hour" ? "按小时" : "按天"}·{" "}
                  {trends.points.length} 个时间桶
                </p>
                <p>
                  更新于 {formatters.updated.format(new Date(trends.updatedAt))}
                </p>
              </div>
            </div>

            <div className="overflow-x-auto rounded-lg border bg-muted/10 px-2 pt-4 pb-2">
              <div
                className="flex h-52 min-w-[38rem] items-end gap-1.5"
                role="group"
                aria-label={`${ranges.find((item) => item.value === trends.window.range)?.label}${selectedMetric.label}趋势`}
              >
                {trends.points.map((point, index) => {
                  const value = pointValue(point, metric)
                  const showTick =
                    index === 0 ||
                    index === trends.points.length - 1 ||
                    index % labelStep === 0
                  return (
                    <button
                      key={point.start}
                      type="button"
                      className="group flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-2 rounded-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                      aria-label={pointLabel(point)}
                      onFocus={() => {
                        focusedIndex.current = index
                        setActiveIndex(index)
                      }}
                      onBlur={() => {
                        focusedIndex.current = null
                      }}
                      onMouseEnter={() => {
                        if (focusedIndex.current === null) setActiveIndex(index)
                      }}
                    >
                      <span className="flex h-40 w-full max-w-8 items-end overflow-hidden rounded-md bg-muted">
                        <span
                          aria-hidden="true"
                          className="w-full rounded-md bg-primary transition-[height,background-color] duration-300 group-hover:bg-primary/80 group-focus-visible:bg-primary/80"
                          style={{
                            height:
                              value === null || value === 0
                                ? "2px"
                                : `${Math.max(4, (value / maximum) * 100)}%`,
                          }}
                        />
                      </span>
                      <span
                        className="h-4 text-[10px] whitespace-nowrap text-muted-foreground"
                        aria-hidden={!showTick}
                      >
                        {showTick
                          ? formatters.tick.format(new Date(point.start))
                          : ""}
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
            <p
              className="min-h-5 text-xs text-muted-foreground"
              aria-live="polite"
            >
              {activePoint
                ? pointLabel(activePoint)
                : "悬停或使用键盘聚焦柱形，可读取该时间段的准确数值。"}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
