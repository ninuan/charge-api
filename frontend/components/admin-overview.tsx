import {
  ActivityIcon,
  ShieldCheckIcon,
  UsersIcon,
  CircleAlertIcon,
} from "lucide-react"

import { AdminIncidentPanel } from "@/components/admin-incident-panel"
import { AdminTrendPanel } from "@/components/admin-trend-panel"
import { MetricCard } from "@/components/metric-card"
import { Card, CardContent } from "@/components/ui/card"
import type { AdminTrendRange, AdminTrendsResponse } from "@/lib/api/generated"
import type { AdminStats } from "@/lib/types"

type AdminOverviewProps = {
  stats: AdminStats | null
  trends: AdminTrendsResponse | null
  trendRange: AdminTrendRange
  trendLoading: boolean
  trendError: string | null
  onTrendRangeChange: (range: AdminTrendRange) => void
  onTrendReload: () => void
  onUser: (id: string) => void
}

export function AdminOverview({
  stats,
  trends,
  trendRange,
  trendLoading,
  trendError,
  onTrendRangeChange,
  onTrendReload,
  onUser,
}: AdminOverviewProps) {
  const overview = stats?.overview

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
      <AdminTrendPanel
        trends={trends}
        requestedRange={trendRange}
        loading={trendLoading}
        error={trendError}
        onRangeChange={onTrendRangeChange}
        onReload={onTrendReload}
      />
      <AdminIncidentPanel initialIssues={stats?.exceptions} onUser={onUser} />
    </div>
  )
}
