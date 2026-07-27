import {
  ActivityIcon,
  BarChart3Icon,
  ShieldCheckIcon,
  UsersIcon,
  CircleAlertIcon,
} from "lucide-react"

import { AdminIncidentPanel } from "@/components/admin-incident-panel"
import { MetricCard } from "@/components/metric-card"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import type { AdminStats, MetricPoint } from "@/lib/types"

type AdminOverviewProps = {
  stats: AdminStats | null
  onUser: (id: string) => void
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

export function AdminOverview({ stats, onUser }: AdminOverviewProps) {
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
        <CardContent className="grid grid-cols-2 p-0 xl:grid-cols-5">
          <MetricCard
            compact
            className="col-span-2 border-b xl:col-span-1 xl:border-r xl:border-b-0"
            tone="warning"
            label="待处理问题"
            value={overview ? overview.openIssues : null}
            detail="当前未解决异常"
            icon={CircleAlertIcon}
          />
          <MetricCard
            compact
            className="border-r border-b xl:border-b-0"
            tone="success"
            label="远端成功率"
            value={overview ? Math.round(overview.remoteSuccessRate) : null}
            suffix="%"
            detail="过去 24 小时远端请求"
            icon={ActivityIcon}
          />
          <MetricCard
            compact
            className="border-b xl:border-r xl:border-b-0"
            tone="primary"
            label="活跃用户"
            value={overview ? overview.activeUsers : null}
            detail="过去 24 小时有访问"
            icon={UsersIcon}
          />
          <MetricCard
            compact
            className="border-r"
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
      <AdminIncidentPanel initialIssues={stats?.exceptions} onUser={onUser} />
    </div>
  )
}
