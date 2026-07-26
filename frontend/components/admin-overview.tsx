import {
  ActivityIcon,
  CircleAlertIcon,
  ShieldCheckIcon,
  UsersIcon,
} from "lucide-react"

import { MetricCard } from "@/components/metric-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import type { AdminHealth, AdminStats } from "@/lib/types"

function healthLabel(state: "healthy" | "degraded" | "unavailable") {
  return state === "healthy" ? "正常" : state === "degraded" ? "异常" : "不可用"
}

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
  health: AdminHealth | null
  onUsers: () => void
}

export function AdminOverview({ stats, health, onUsers }: AdminOverviewProps) {
  const overview = stats?.overview

  return (
    <div className="flex flex-col gap-4">
      <Card className="shadow-xs" aria-label="运营指标">
        <CardContent className="grid divide-y p-0 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-5">
          <MetricCard
            compact
            tone="warning"
            label="待处理问题"
            value={overview ? overview.openIssues : null}
            detail="需要关注的异常"
            icon={CircleAlertIcon}
          />
          <MetricCard
            compact
            tone="success"
            label="远端成功率"
            value={overview ? Math.round(overview.remoteSuccessRate) : null}
            detail="近期远端请求成功率（%）"
            icon={ActivityIcon}
          />
          <MetricCard
            compact
            tone="primary"
            label="活跃用户"
            value={overview ? overview.activeUsers : null}
            detail="近期访问过的账户"
            icon={UsersIcon}
          />
          <MetricCard
            compact
            label="管理设备"
            value={overview ? overview.managedDevices : null}
            detail="系统中的充电桩"
            icon={ShieldCheckIcon}
          />
          <MetricCard
            compact
            tone="destructive"
            label="离线端口"
            value={overview ? overview.offlinePorts : null}
            detail="需要复查设备状态"
            icon={CircleAlertIcon}
          />
        </CardContent>
      </Card>
      <Card className="shadow-xs">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">服务健康</CardTitle>
          <p className="text-xs text-muted-foreground">
            服务状态仅描述可用性，不会暴露上游连接细节。
          </p>
        </CardHeader>
        <CardContent className="grid gap-2 sm:grid-cols-3">
          {(["charge", "database", "yyb"] as const).map((name) => (
            <div
              key={name}
              className="flex items-center justify-between gap-3 rounded-lg border p-3"
            >
              <div>
                <p className="text-sm font-medium">
                  {
                    { charge: "充电服务", database: "数据库", yyb: "扫码服务" }[
                      name
                    ]
                  }
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {health?.[name].message ?? "正在检查服务状态"}
                </p>
              </div>
              <Badge
                variant={
                  health?.[name].state === "healthy"
                    ? "secondary"
                    : "destructive"
                }
              >
                {health ? healthLabel(health[name].state) : "检查中"}
              </Badge>
            </div>
          ))}
        </CardContent>
      </Card>
      <Card className="shadow-xs">
        <CardHeader className="flex-row items-center justify-between pb-3">
          <div>
            <CardTitle className="text-base">最近异常</CardTitle>
            <p className="mt-1 text-xs text-muted-foreground">
              管理员诊断已脱敏，不含登录凭据、会话或上游地址。
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={onUsers}>
            筛选用户
          </Button>
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
                  <p className="text-sm font-medium">
                    {issue.username} · {issueTypeLabel(issue.type)}
                  </p>
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
