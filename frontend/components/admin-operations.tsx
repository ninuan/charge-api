"use client"

import {
  ArchiveIcon,
  BellRingIcon,
  CheckCircle2Icon,
  Clock3Icon,
  DatabaseIcon,
  FileClockIcon,
  GaugeIcon,
  HardDriveIcon,
  HistoryIcon,
  MessageCircleIcon,
  RefreshCwIcon,
  SendIcon,
  ShieldCheckIcon,
  TimerResetIcon,
  TriangleAlertIcon,
  UsersIcon,
} from "lucide-react"
import { useEffect, useState } from "react"
import { toast } from "sonner"

import { Badge } from "@/components/ui/badge"
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
import { adminApi } from "@/lib/admin-api"
import type { AuditPage, OperationsStatus } from "@/lib/types"
import { cn } from "@/lib/utils"

const actionLabels: Record<string, string> = {
  "user.create": "创建用户",
  "user.delete": "删除用户",
  "user.password_reset": "重置密码",
  "user.enabled_update": "修改账户状态",
  "user.role_update": "修改用户角色",
  "user.device_limit_update": "修改设备额度",
  "user.refresh_policy_update": "修改刷新策略",
  "user.refresh": "强制刷新用户设备",
  "settings.update": "修改系统设置",
  "invite.create": "创建邀请码",
  "invite.delete": "删除邀请码",
  "incident.update": "处理异常",
}

function formatBytes(value?: number) {
  if (!value) return "0 B"
  const units = ["B", "KB", "MB", "GB"]
  let current = value
  let unit = 0
  while (current >= 1024 && unit < units.length - 1) {
    current /= 1024
    unit++
  }
  return `${current >= 10 || unit === 0 ? current.toFixed(0) : current.toFixed(1)} ${units[unit]}`
}

function formatHistoryPeriod(operations: OperationsStatus) {
  const prefix = `保留 ${operations.portHistoryRetentionDays} 天`
  if (!operations.portHistoryOldestAt || !operations.portHistoryNewestAt) {
    return `${prefix} · 尚无状态记录`
  }
  const formatter = new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
  return `${prefix} · ${formatter.format(new Date(operations.portHistoryOldestAt))} 至 ${formatter.format(new Date(operations.portHistoryNewestAt))}`
}

const reminderStateLabels = {
  healthy: "运行正常",
  degraded: "需要处理",
  disabled: "已停用",
  power_off: "计划暂停",
  stopped: "未运行",
} as const

const wxPusherStateLabels = {
  healthy: "运行正常",
  degraded: "需要处理",
  disabled: "未配置",
  stopped: "未运行",
} as const

const wxPusherErrorLabels = {
  configuration: "应用配置",
  rate_limited: "供应商限流",
  provider_unavailable: "供应商不可用",
  delivery_unconfirmed: "投递未确认",
  binding_invalid: "用户绑定失效",
  internal: "内部处理",
} as const

function formatNextAttempt(value?: string) {
  if (!value) return "暂无任务"
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function formatTemporaryMinutes(value: number) {
  if (!value) return "暂无样本"
  if (value < 60) return `${Math.round(value)} 分钟`
  return `${(value / 60).toFixed(value >= 600 ? 0 : 1)} 小时`
}

function formatOperationTime(value?: string) {
  if (!value) return "暂无记录"
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function AdminOperations() {
  const [operations, setOperations] = useState<OperationsStatus | null>(null)
  const [audit, setAudit] = useState<AuditPage | null>(null)
  const [checking, setChecking] = useState(false)

  async function recheck() {
    setChecking(true)
    try {
      const [nextOperations, nextAudit] = await Promise.all([
        adminApi.operations(),
        adminApi.audit(),
      ])
      setOperations(nextOperations)
      setAudit(nextAudit)
      toast.success("运维状态已重新检查")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setChecking(false)
    }
  }

  useEffect(() => {
    void Promise.all([adminApi.operations(), adminApi.audit()])
      .then(([nextOperations, nextAudit]) => {
        setOperations(nextOperations)
        setAudit(nextAudit)
      })
      .catch((reason) => toast.error((reason as Error).message))
  }, [])

  const cards = operations
    ? [
        {
          label: "数据库大小",
          value: formatBytes(operations.databaseSizeBytes),
          detail: `完整性校验：${operations.integrityResult}`,
          icon: DatabaseIcon,
        },
        {
          label: "指标数据",
          value: `${operations.metricRows.toLocaleString("zh-CN")} 条`,
          detail: `保留 ${operations.metricRetentionDays} 天`,
          icon: HardDriveIcon,
        },
        {
          label: "端口历史",
          value: `${operations.portHistoryRows.toLocaleString("zh-CN")} 条`,
          detail: formatHistoryPeriod(operations),
          icon: HistoryIcon,
        },
        {
          label: "站内通知",
          value: `${operations.notificationRows.toLocaleString("zh-CN")} 条`,
          detail: `已解决 ${operations.resolvedNotificationRows.toLocaleString("zh-CN")} 条 · 保留 ${operations.notificationRetentionDays} 天`,
          icon: BellRingIcon,
        },
        {
          label: "最近备份",
          value: operations.lastBackupAt
            ? new Date(operations.lastBackupAt).toLocaleDateString("zh-CN")
            : "尚未发现",
          detail: operations.lastBackupAt
            ? `${new Date(operations.lastBackupAt).toLocaleTimeString("zh-CN")} · ${formatBytes(operations.lastBackupSizeBytes)}`
            : operations.backupMessage,
          icon: ArchiveIcon,
        },
        {
          label: "最近校验",
          value: operations.integrityResult === "ok" ? "通过" : "需检查",
          detail: new Date(operations.checkedAt).toLocaleString("zh-CN"),
          icon:
            operations.integrityResult === "ok"
              ? CheckCircle2Icon
              : TriangleAlertIcon,
        },
      ]
    : []

  return (
    <div className="grid gap-4">
      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2 text-base">
            <BellRingIcon className="size-4 text-primary" />
            提醒调度
            {operations && (
              <Badge
                variant={
                  operations.reminders.state === "degraded"
                    ? "destructive"
                    : "secondary"
                }
              >
                {reminderStateLabels[operations.reminders.state]}
              </Badge>
            )}
          </CardTitle>
          <CardDescription>
            后台按整桩低频刷新；计划断电期间暂停请求和离线提醒。
          </CardDescription>
          <CardAction>
            <Button
              size="sm"
              variant="outline"
              disabled={checking}
              onClick={() => void recheck()}
            >
              <RefreshCwIcon
                data-icon="inline-start"
                className={cn(checking && "motion-safe:animate-spin")}
              />
              {checking ? "检查中…" : "重新检查"}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-3">
          {operations ? (
            <>
              <Alert
                variant={
                  operations.reminders.state === "degraded"
                    ? "destructive"
                    : "default"
                }
              >
                <BellRingIcon />
                <AlertTitle>当前状态</AlertTitle>
                <AlertDescription>
                  {operations.reminders.message}
                </AlertDescription>
              </Alert>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {[
                  {
                    label: "活动提醒",
                    value: `${operations.reminders.activeTemporaryRules + operations.reminders.activeRecurringRules} 条`,
                    detail: `临时 ${operations.reminders.activeTemporaryRules} · 固定 ${operations.reminders.activeRecurringRules}（全局${operations.reminders.recurringEnabled ? "开启" : "关闭"}）`,
                    icon: BellRingIcon,
                  },
                  {
                    label: "实际检查充电桩",
                    value: `${operations.reminders.remoteAttempts24Hours} 次`,
                    detail: `覆盖 ${operations.reminders.trackedPiles} 台 · 成功 ${operations.reminders.remoteSuccesses24Hours} · 失败 ${operations.reminders.remoteFailures24Hours}`,
                    icon: GaugeIcon,
                  },
                  {
                    label: "24 小时请求成功率",
                    value: operations.reminders.remoteAttempts24Hours
                      ? `${operations.reminders.remoteSuccessRate24Hours.toFixed(1)}%`
                      : "暂无请求",
                    detail: `${operations.reminders.remoteSuccesses24Hours}/${operations.reminders.remoteAttempts24Hours} 次成功 · ${operations.reminders.remoteFailures24Hours} 次失败`,
                    icon: GaugeIcon,
                  },
                  {
                    label: "请求复用与保护",
                    value: `${operations.reminders.cacheHits24Hours + operations.reminders.coalesced24Hours} 次`,
                    detail: `缓存 ${operations.reminders.cacheHits24Hours} · 合并 ${operations.reminders.coalesced24Hours} · 额度跳过 ${operations.reminders.quotaSkips24Hours}`,
                    icon: ShieldCheckIcon,
                  },
                  {
                    label: "临时提醒结果",
                    value: `${operations.reminders.completedNotified24Hours} 条自动完成`,
                    detail: `${operations.reminders.completedExpired24Hours} 条到期结束 · 无需继续查询`,
                    icon: CheckCircle2Icon,
                  },
                  {
                    label: "临时提醒平均时长",
                    value: formatTemporaryMinutes(
                      operations.reminders.averageTemporaryMinutes
                    ),
                    detail: "按过去 24 小时已结束任务计算",
                    icon: TimerResetIcon,
                  },
                ].map(({ label, value, detail, icon: Icon }) => (
                  <div
                    key={label}
                    className="rounded-xl border bg-muted/20 p-4"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-xs text-muted-foreground">{label}</p>
                      <Icon className="size-4 text-primary" />
                    </div>
                    <p className="mt-2 text-base font-semibold tabular-nums">
                      {value}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {detail}
                    </p>
                  </div>
                ))}
              </div>
              {(operations.reminders.schedulerErrors24Hours > 0 ||
                operations.reminders.maxConsecutiveFailures > 0) && (
                <p className="text-xs text-muted-foreground">
                  24 小时调度异常 {operations.reminders.schedulerErrors24Hours}{" "}
                  次 · 单桩最大连续失败{" "}
                  {operations.reminders.maxConsecutiveFailures} 次
                </p>
              )}
              <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                <Clock3Icon className="size-3.5" />
                下次任务：
                {formatNextAttempt(operations.reminders.nextAttemptAt)}
                <span aria-hidden="true">·</span>
                {operations.reminders.scheduledPowerOffActive
                  ? "等待计划供电恢复"
                  : `待执行 ${operations.reminders.duePiles} 台，执行中 ${operations.reminders.inFlightPiles} 台`}
              </p>
            </>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {Array.from({ length: 6 }, (_, index) => (
                <Skeleton key={index} className="h-28 w-full" />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2 text-base">
            <MessageCircleIcon className="size-4 text-primary" />
            WxPusher 投递
            {operations && (
              <Badge
                variant={
                  operations.wxPusher.state === "degraded" ||
                  operations.wxPusher.state === "stopped"
                    ? "destructive"
                    : "secondary"
                }
              >
                {wxPusherStateLabels[operations.wxPusher.state]}
              </Badge>
            )}
          </CardTitle>
          <CardDescription>
            查看投递处理进度和需要单独处理的用户绑定问题。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {operations ? (
            <>
              <Alert
                variant={
                  operations.wxPusher.state === "degraded" ||
                  operations.wxPusher.state === "stopped"
                    ? "destructive"
                    : "default"
                }
              >
                <SendIcon />
                <AlertTitle>通道状态</AlertTitle>
                <AlertDescription>
                  {operations.wxPusher.message}
                </AlertDescription>
              </Alert>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {[
                  {
                    label: "活动绑定",
                    value: `${operations.wxPusher.activeBindings} 位`,
                    detail: operations.wxPusher.configured
                      ? `投递调度器${operations.wxPusher.dispatcherRunning ? "运行中" : "未运行"}`
                      : "未配置 AppToken",
                    icon: UsersIcon,
                  },
                  {
                    label: "当前投递队列",
                    value: `${operations.wxPusher.pendingDeliveries + operations.wxPusher.sendingDeliveries + operations.wxPusher.acceptedPendingDeliveries + operations.wxPusher.retryingDeliveries} 条`,
                    detail: `待发 ${operations.wxPusher.pendingDeliveries} · 发送中 ${operations.wxPusher.sendingDeliveries} · 待确认 ${operations.wxPusher.acceptedPendingDeliveries} · 重试 ${operations.wxPusher.retryingDeliveries}`,
                    icon: SendIcon,
                  },
                  {
                    label: "24 小时服务受理",
                    value: operations.wxPusher.attempts24Hours
                      ? `${operations.wxPusher.acceptanceRate24Hours.toFixed(1)}%`
                      : "暂无投递",
                    detail: `${operations.wxPusher.accepted24Hours}/${operations.wxPusher.attempts24Hours} 条已受理`,
                    icon: CheckCircle2Icon,
                  },
                  {
                    label: "WxPusher 已处理",
                    value: operations.wxPusher.accepted24Hours
                      ? `${operations.wxPusher.providerSuccessRate24Hours.toFixed(1)}%`
                      : "暂无样本",
                    detail: `${operations.wxPusher.providerSucceeded24Hours}/${operations.wxPusher.accepted24Hours} 条状态确认成功`,
                    icon: GaugeIcon,
                  },
                  {
                    label: "24 小时异常",
                    value: `${operations.wxPusher.systemFailures24Hours} 次系统异常`,
                    detail: `绑定异常 ${operations.wxPusher.bindingFailures24Hours} 次 / ${operations.wxPusher.affectedBindingUsers} 位用户`,
                    icon: TriangleAlertIcon,
                  },
                  {
                    label: "最近处理成功",
                    value: formatOperationTime(
                      operations.wxPusher.lastProviderSuccessAt
                    ),
                    detail: operations.wxPusher.lastErrorCategory
                      ? `最近错误类别：${wxPusherErrorLabels[operations.wxPusher.lastErrorCategory]}`
                      : `最近受理：${formatOperationTime(operations.wxPusher.lastAcceptedAt)}`,
                    icon: HistoryIcon,
                  },
                ].map(({ label, value, detail, icon: Icon }) => (
                  <div
                    key={label}
                    className="rounded-xl border bg-muted/20 p-4"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-xs text-muted-foreground">{label}</p>
                      <Icon className="size-4 text-primary" />
                    </div>
                    <p className="mt-2 text-base font-semibold tabular-nums">
                      {value}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {detail}
                    </p>
                  </div>
                ))}
              </div>
              {(operations.wxPusher.uncertainDeliveries > 0 ||
                operations.wxPusher.failedDeliveries > 0 ||
                operations.wxPusher.oldestPendingAt) && (
                <p className="text-xs text-muted-foreground">
                  未确认 {operations.wxPusher.uncertainDeliveries} 条 · 失败{" "}
                  {operations.wxPusher.failedDeliveries} 条 · 最早排队{" "}
                  {formatOperationTime(operations.wxPusher.oldestPendingAt)}
                </p>
              )}
              {operations.wxPusher.lastFailureAt && (
                <p className="text-xs text-muted-foreground">
                  最近失败：
                  {operations.wxPusher.lastErrorCategory
                    ? wxPusherErrorLabels[operations.wxPusher.lastErrorCategory]
                    : "未分类"}
                  ，发生于{" "}
                  {formatOperationTime(operations.wxPusher.lastFailureAt)}
                </p>
              )}
            </>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {Array.from({ length: 6 }, (_, index) => (
                <Skeleton key={index} className="h-28 w-full" />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheckIcon className="size-4 text-primary" />
            数据与备份
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {operations
            ? cards.map(({ label, value, detail, icon: Icon }) => (
                <div key={label} className="rounded-xl border bg-muted/20 p-4">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-xs text-muted-foreground">{label}</p>
                    <Icon className="size-4 text-primary" />
                  </div>
                  <p className="mt-2 text-lg font-semibold tabular-nums">
                    {value}
                  </p>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {detail}
                  </p>
                </div>
              ))
            : Array.from({ length: 6 }, (_, index) => (
                <Skeleton key={index} className="h-28 w-full" />
              ))}
          {operations && operations.backupState !== "healthy" && (
            <div className="sm:col-span-2 xl:col-span-3">
              <p className="flex items-start gap-2 rounded-lg bg-warning/10 p-3 text-xs leading-5 text-warning-foreground">
                <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
                {operations?.backupMessage}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="shadow-xs">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <FileClockIcon className="size-4 text-primary" />
            管理操作日志
            {audit && <Badge variant="secondary">{audit.total} 条</Badge>}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-2">
          {!audit ? (
            <>
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </>
          ) : audit.items.length ? (
            audit.items.map((entry) => (
              <div
                key={entry.id}
                className="grid gap-1 rounded-lg border p-3 transition-colors duration-150 hover:bg-muted/25 sm:grid-cols-[1fr_auto] sm:items-center"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-sm font-medium">
                      {entry.actor} ·
                      {actionLabels[entry.action] ?? entry.action}
                    </p>
                    <Badge
                      variant={
                        entry.result === "success" ? "secondary" : "destructive"
                      }
                    >
                      {entry.result === "success" ? "成功" : "失败"}
                    </Badge>
                  </div>
                  <p className="mt-1 truncate text-xs text-muted-foreground">
                    对象：{entry.targetLabel || entry.targetId || "系统"}
                  </p>
                </div>
                <time className="text-xs text-muted-foreground">
                  {new Date(entry.createdAt).toLocaleString("zh-CN")}
                </time>
              </div>
            ))
          ) : (
            <p className="py-8 text-center text-sm text-muted-foreground">
              暂无管理员操作记录。
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
