"use client"

import {
  CircleCheckIcon,
  CircleXIcon,
  DatabaseIcon,
  LoaderCircleIcon,
  QrCodeIcon,
  ServerIcon,
  TriangleAlertIcon,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import type { AdminHealth, HealthState } from "@/lib/types"

const services = [
  { key: "charge", label: "充电服务", icon: ServerIcon },
  { key: "database", label: "数据库", icon: DatabaseIcon },
  { key: "yyb", label: "扫码服务", icon: QrCodeIcon },
] as const

const statePresentation: Record<
  HealthState,
  {
    label: string
    icon: typeof CircleCheckIcon
    iconClassName: string
    badgeClassName: string
  }
> = {
  healthy: {
    label: "正常",
    icon: CircleCheckIcon,
    iconClassName: "bg-success/15 text-success-foreground",
    badgeClassName: "bg-success/15 text-success-foreground",
  },
  degraded: {
    label: "异常",
    icon: TriangleAlertIcon,
    iconClassName: "bg-warning/15 text-warning-foreground",
    badgeClassName: "bg-warning/15 text-warning-foreground",
  },
  unavailable: {
    label: "不可用",
    icon: CircleXIcon,
    iconClassName: "bg-destructive/10 text-destructive",
    badgeClassName: "bg-destructive/10 text-destructive",
  },
}

function healthSummary(health: AdminHealth | null) {
  if (!health) {
    return {
      label: "检查中",
      dotClassName: "bg-muted-foreground/40 motion-safe:animate-pulse",
    }
  }

  const states = services.map(({ key }) => health[key].state)
  const healthyCount = states.filter((state) => state === "healthy").length

  if (states.includes("unavailable")) {
    return {
      label: `${healthyCount}/3 正常`,
      dotClassName: "bg-destructive motion-safe:animate-pulse",
    }
  }
  if (states.includes("degraded")) {
    return {
      label: `${healthyCount}/3 正常`,
      dotClassName: "bg-warning motion-safe:animate-pulse",
    }
  }
  return {
    label: "全部正常",
    dotClassName: "bg-success motion-safe:animate-pulse",
  }
}

export function AdminHealthStatus({ health }: { health: AdminHealth | null }) {
  const summary = healthSummary(health)

  return (
    <Dialog>
      <DialogTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="justify-start px-2 text-muted-foreground hover:text-foreground"
            aria-label={`服务健康：${summary.label}，查看详情`}
          />
        }
      >
        <span
          aria-hidden="true"
          className={`size-1.5 shrink-0 rounded-full ${summary.dotClassName}`}
        />
        <span aria-live="polite">
          服务<span className="hidden md:inline">健康：</span>
          <span className="md:hidden"> </span>
          {summary.label}
        </span>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>服务健康</DialogTitle>
          <DialogDescription>
            {health?.checkedAt
              ? `最近检查于 ${new Date(health.checkedAt).toLocaleString("zh-CN")}。`
              : "正在获取最新服务状态。"}
            状态详情不会暴露上游连接地址或凭据。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2" aria-label="服务健康详情">
          {services.map(({ key, label, icon: ServiceIcon }) => {
            const service = health?.[key]
            const presentation = service
              ? statePresentation[service.state]
              : null
            const StateIcon = presentation?.icon ?? LoaderCircleIcon

            return (
              <div
                key={key}
                className="grid grid-cols-[auto_1fr_auto] items-center gap-3 rounded-lg border p-3"
              >
                <span
                  className={`grid size-9 place-items-center rounded-lg ${
                    presentation?.iconClassName ??
                    "bg-muted text-muted-foreground"
                  }`}
                >
                  <ServiceIcon className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-medium">{label}</p>
                  <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                    {service?.message ?? "正在检查服务状态"}
                  </p>
                </div>
                <Badge
                  variant="secondary"
                  className={presentation?.badgeClassName}
                >
                  <StateIcon
                    className={
                      presentation ? undefined : "motion-safe:animate-spin"
                    }
                  />
                  {presentation?.label ?? "检查中"}
                </Badge>
              </div>
            )
          })}
        </div>
      </DialogContent>
    </Dialog>
  )
}
