import {
  ActivityIcon,
  BarChart3Icon,
  CircleAlertIcon,
  ShieldCheckIcon,
  UsersIcon,
} from "lucide-react"

import { MetricCard } from "@/components/metric-card"
import { Badge } from "@/components/ui/badge"
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
import type { AdminStats, MetricPoint } from "@/lib/types"

function issueTypeLabel(type: string) {
  return (
    (
      {
        credential: "凭据",
        cookie_expired: "凭据失效",
        refresh: "刷新",
        stale: "数据滞后",
        offline: "离线端口",
        operation: "用户操作",
      } as Record<string, string>
    )[type] ?? "系统"
  )
}

type AdminOverviewProps = {
  stats: AdminStats | null
  onUsers: () => void
}

function TrendPanel({
  label,
  detail,
  points,
  value,
  summary,
  ariaSummary,
}: {
  label: string
  detail: string
  points: MetricPoint[]
  value: (point: MetricPoint) => number
  summary: string
  ariaSummary: string
}) {
  const values = points.map(value)
  const maximum = Math.max(1, ...values)

  return (
    <div className="rounded-lg border p-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium">{label}</p>
          <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
        </div>
        <p className="text-lg font-semibold whitespace-nowrap tabular-nums">
          {summary}
        </p>
      </div>
      {values.length ? (
        <div
          className="mt-4 flex h-20 items-end gap-1 motion-safe:animate-in motion-safe:duration-300 motion-safe:fade-in motion-safe:slide-in-from-bottom-1"
          role="img"
          aria-label={`${label}趋势，${ariaSummary}`}
        >
          {values.map((current, index) => (
            <span
              // 时序点可能共享时间格式，索引可稳定表达本次已排序的序列。
              key={`${points[index]?.time}-${index}`}
              className="min-w-0 flex-1 rounded-sm bg-primary/70"
              style={{
                height:
                  current === 0
                    ? "2px"
                    : `${Math.max(6, (current / maximum) * 100)}%`,
              }}
              title={`${points[index]?.time}：${current}`}
              aria-hidden="true"
            />
          ))}
        </div>
      ) : (
        <p className="mt-4 grid h-20 place-items-center rounded-md bg-muted/35 text-xs text-muted-foreground">
          暂无趋势数据
        </p>
      )}
    </div>
  )
}

export function AdminOverview({ stats, onUsers }: AdminOverviewProps) {
  const overview = stats?.overview
  const hourlyRemote = stats?.hourly.reduce(
    (total, point) => total + point.remote,
    0
  )
  const hourlyRemoteOk = stats?.hourly.reduce(
    (total, point) => total + point.remoteOk,
    0
  )
  const hourlySuccessRate = hourlyRemote
    ? Math.round(((hourlyRemoteOk ?? 0) / hourlyRemote) * 100)
    : 0
  const dailyActivePeak = stats?.daily.reduce(
    (maximum, point) => Math.max(maximum, point.activeUsers),
    0
  )

  return (
    <div className="flex flex-col gap-4">
      <Card className="shadow-xs" aria-label="运营指标">
        <CardContent className="grid divide-y p-0 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-5">
          <MetricCard
            compact
            tone="warning"
            label="待处理问题"
            value={overview ? overview.openIssues : null}
            detail="当前未解决异常"
            icon={CircleAlertIcon}
          />
          <MetricCard
            compact
            tone="success"
            label="远端成功率"
            value={overview ? Math.round(overview.remoteSuccessRate) : null}
            suffix="%"
            detail="过去 24 小时远端请求"
            icon={ActivityIcon}
          />
          <MetricCard
            compact
            tone="primary"
            label="活跃用户"
            value={overview ? overview.activeUsers : null}
            detail="过去 24 小时有访问"
            icon={UsersIcon}
          />
          <MetricCard
            compact
            label="管理设备"
            value={overview ? overview.managedDevices : null}
            detail="普通用户绑定设备"
            icon={ShieldCheckIcon}
          />
          <MetricCard
            compact
            tone="destructive"
            label="离线端口"
            value={overview ? overview.offlinePorts : null}
            detail="当前快照需要复查"
            icon={CircleAlertIcon}
          />
        </CardContent>
      </Card>
      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <BarChart3Icon className="size-4 text-primary" />
            运营趋势
          </CardTitle>
          <CardDescription className="text-xs">
            分别观察短期远端请求量与长期活跃账户变化。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 lg:grid-cols-2">
          {!stats ? (
            <>
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-32 w-full" />
            </>
          ) : (
            <>
              <TrendPanel
                label={`过去 ${stats.hourly.length} 小时远端请求`}
                detail={`请求成功率 ${hourlySuccessRate}%`}
                points={stats.hourly}
                value={(point) => point.remote}
                summary={`${hourlyRemote ?? 0} 次`}
                ariaSummary={`共 ${hourlyRemote ?? 0} 次，成功率 ${hourlySuccessRate}%`}
              />
              <TrendPanel
                label={`过去 ${stats.daily.length} 天活跃用户`}
                detail="每日活跃账户数"
                points={stats.daily}
                value={(point) => point.activeUsers}
                summary={`峰值 ${dailyActivePeak ?? 0} 人`}
                ariaSummary={`单日峰值 ${dailyActivePeak ?? 0} 人`}
              />
            </>
          )}
        </CardContent>
      </Card>
      <Card className="shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            最近异常
            {stats ? (
              <Badge
                variant={stats.exceptions.length ? "destructive" : "secondary"}
              >
                {stats.exceptions.length} 条
              </Badge>
            ) : null}
          </CardTitle>
          <CardDescription className="text-xs">
            管理员诊断已脱敏，不含登录凭据、会话或上游地址。
          </CardDescription>
          <CardAction>
            <Button variant="outline" size="sm" onClick={onUsers}>
              筛选用户
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {!stats ? (
            <>
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </>
          ) : stats.exceptions?.length ? (
            stats.exceptions.slice(0, 6).map((issue) => (
              <div
                key={issue.id}
                className="flex flex-col gap-1 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-sm font-medium">
                      {issue.username} · {issueTypeLabel(issue.type)}
                    </p>
                    <Badge
                      variant={
                        issue.level === "critical" ? "destructive" : "secondary"
                      }
                      className={
                        issue.level === "critical"
                          ? undefined
                          : "bg-warning/15 text-warning-foreground"
                      }
                    >
                      {issue.level === "critical" ? "紧急" : "提醒"}
                    </Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {issue.message}
                    {issue.deviceId ? ` · 设备尾号 ${issue.deviceId}` : ""}
                  </p>
                </div>
                <p className="text-xs text-muted-foreground">
                  {new Date(issue.time).toLocaleString("zh-CN")}
                </p>
              </div>
            ))
          ) : (
            <p className="py-4 text-sm text-muted-foreground">暂无近期异常。</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
