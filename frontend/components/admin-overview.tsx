import { AdminIncidentPanel } from "@/components/admin-incident-panel"
import { AdminTrendPanel } from "@/components/admin-trend-panel"
import type { AdminTrendRange, AdminTrendsResponse } from "@/lib/api/generated"
import type { AdminStats } from "@/lib/types"

type Props = {
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
}: Props) {
  const overview = stats?.overview
  return (
    <div className="wb-admin-overview wb-continuous">
      <section className="wb-admin-priority">
        <AdminIncidentPanel
          initialIssues={stats?.exceptions}
          onUser={onUser}
          compact
        />
      </section>
      <section className="wb-admin-trends">
        <div className="wb-section-heading">
          <div>
            <h2>请求与使用</h2>
            <p>运行情况与趋势，帮助判断服务是否稳定。</p>
          </div>
        </div>
        <dl className="wb-admin-overview-facts">
          {[
            ["活跃用户", overview?.activeUsers],
            ["管理设备", overview?.managedDevices],
            [
              "远端成功率",
              overview
                ? `${Math.round(overview.remoteSuccessRate)}%`
                : undefined,
            ],
            ["离线端口", overview?.offlinePorts],
          ].map(([label, value]) => (
            <div key={label} aria-label={`${label} 指标`}>
              <dt>{label}</dt>
              <dd>{value ?? "—"}</dd>
            </div>
          ))}
        </dl>
        <AdminTrendPanel
          trends={trends}
          requestedRange={trendRange}
          loading={trendLoading}
          error={trendError}
          onRangeChange={onTrendRangeChange}
          onReload={onTrendReload}
        />
      </section>
    </div>
  )
}
