"use client"

import {
  BellRingIcon,
  CheckCircle2Icon,
  Clock3Icon,
  GaugeIcon,
  HistoryIcon,
  LoaderCircleIcon,
  MoonIcon,
  PlusIcon,
  PowerOffIcon,
  RotateCwIcon,
  Settings2Icon,
  Trash2Icon,
  XCircleIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { notify } from "@/lib/feedback"

import type { WatchEditorTarget } from "@/components/watch-rule-dialog"
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type {
  NotificationPreference,
  NotificationPreferenceUpdateRequest,
  WatchRule,
  WatchTemporaryDuration,
} from "@/lib/api/generated"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import {
  formatCompletionReason,
  formatNextCheck,
  formatPowerWindow,
  formatRemainingTime,
  formatWatchTimestamp,
  getTemporaryDurationOptions,
  minutesToTime,
  timeToMinutes,
} from "@/lib/watch-format"

function pileLabel(piles: Pile[], deviceId: string) {
  const pile = piles.find((candidate) => candidate.id === deviceId)
  return pile?.name || pile?.number || deviceId
}

function ruleLabel() {
  return "整桩任意端口空闲时提醒"
}

const recurringRetirementNoticeKey = "charge:recurring-reminders-retired:v1"

export function QuietHoursForm({
  preference,
  onSave,
}: {
  preference: NotificationPreference
  onSave: (
    payload: NotificationPreferenceUpdateRequest
  ) => Promise<NotificationPreference>
}) {
  const [saving, setSaving] = useState(false)
  const [enabled, setEnabled] = useState(preference.quietHoursEnabled)
  const [start, setStart] = useState(minutesToTime(preference.quietStartMinute))
  const [end, setEnd] = useState(minutesToTime(preference.quietEndMinute))

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const startMinute = timeToMinutes(start)
    const endMinute = timeToMinutes(end)
    if (startMinute == null || endMinute == null) {
      notify.warning("请选择有效的免打扰时段", {
        id: "quiet-hours-validation",
      })
      return
    }
    setSaving(true)
    try {
      await onSave({
        quietHoursEnabled: enabled,
        quietStartMinute: startMinute,
        quietEndMinute: endMinute,
        timezone: "Asia/Shanghai",
      })
      notify.success("免打扰设置已保存")
    } catch (reason) {
      notify.error(reason, { title: "保存免打扰设置失败" })
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="quiet-enabled">
              <MoonIcon className="size-4" />
              启用免打扰
            </FieldLabel>
            <FieldDescription>
              开启后，这段时间不弹出网页或微信提醒，消息仍会保留在通知中心。
            </FieldDescription>
          </FieldContent>
          <Switch
            id="quiet-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        </Field>
        <FieldGroup className="grid gap-4 sm:grid-cols-2">
          <Field data-disabled={!enabled}>
            <FieldLabel htmlFor="quiet-start">开始时间</FieldLabel>
            <Input
              id="quiet-start"
              type="time"
              value={start}
              disabled={!enabled}
              onChange={(event) => setStart(event.target.value)}
            />
          </Field>
          <Field data-disabled={!enabled}>
            <FieldLabel htmlFor="quiet-end">结束时间</FieldLabel>
            <Input
              id="quiet-end"
              type="time"
              value={end}
              disabled={!enabled}
              onChange={(event) => setEnd(event.target.value)}
            />
          </Field>
        </FieldGroup>
        <Button type="submit" disabled={saving}>
          {saving ? (
            <LoaderCircleIcon
              data-icon="inline-start"
              className="motion-safe:animate-spin"
            />
          ) : null}
          {saving ? "保存中…" : "保存免打扰设置"}
        </Button>
      </FieldGroup>
    </form>
  )
}

export function WatchManagementSheet({
  piles,
  open,
  onOpenChange,
  onEditRule,
}: {
  piles: Pile[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onEditRule: (target: WatchEditorTarget) => void
}) {
  const {
    rules,
    overview,
    preference,
    loading,
    loaded,
    error,
    load,
    updateRule,
    deleteRule,
    updatePreference,
  } = useWatch()
  const [pendingRuleIds, setPendingRuleIds] = useState<Set<string>>(
    () => new Set()
  )
  const [cancelCandidate, setCancelCandidate] = useState<WatchRule | null>(null)
  const [deleteCandidate, setDeleteCandidate] = useState<WatchRule | null>(null)
  const [extendCandidate, setExtendCandidate] = useState<WatchRule | null>(null)
  const [extendDuration, setExtendDuration] =
    useState<WatchTemporaryDuration>("4h")
  const availableDurationOptions = useMemo(
    () =>
      getTemporaryDurationOptions(Boolean(overview?.scheduledPowerOffEnabled)),
    [overview?.scheduledPowerOffEnabled]
  )
  const [now, setNow] = useState(() => Date.now())
  const [
    recurringRetirementNoticeDismissed,
    setRecurringRetirementNoticeDismissed,
  ] = useState(
    () =>
      typeof window !== "undefined" &&
      window.localStorage.getItem(recurringRetirementNoticeKey) === "1"
  )

  useEffect(() => {
    if (open && !loaded && !loading) {
      void load().catch(() => undefined)
    }
  }, [load, loaded, loading, open])

  useEffect(() => {
    if (!open) return
    const initialTimer = window.setTimeout(() => setNow(Date.now()), 0)
    const timer = window.setInterval(() => setNow(Date.now()), 60_000)
    return () => {
      window.clearTimeout(initialTimer)
      window.clearInterval(timer)
    }
  }, [open])

  const orderedRules = useMemo(
    () =>
      rules.toSorted((left, right) => {
        if (left.completedAt !== right.completedAt) {
          return left.completedAt ? 1 : -1
        }
        return right.updatedAt.localeCompare(left.updatedAt)
      }),
    [rules]
  )
  const activeTemporaryRules = orderedRules.filter(
    (rule) => rule.mode === "temporary" && !rule.completedAt
  )
  const completedTemporaryRules = orderedRules.filter(
    (rule) => rule.mode === "temporary" && Boolean(rule.completedAt)
  )
  const recurringRules = orderedRules.filter(
    (rule) => rule.mode === "recurring"
  )

  function setRulePending(ruleId: string, pending: boolean) {
    setPendingRuleIds((current) => {
      const next = new Set(current)
      if (pending) next.add(ruleId)
      else next.delete(ruleId)
      return next
    })
  }

  function dismissRecurringRetirementNotice() {
    window.localStorage.setItem(recurringRetirementNoticeKey, "1")
    setRecurringRetirementNoticeDismissed(true)
  }

  async function confirmCancel() {
    if (!cancelCandidate) return
    const candidate = cancelCandidate
    setRulePending(candidate.id, true)
    try {
      await updateRule(candidate.id, { cancel: true })
      setCancelCandidate(null)
      notify.success("临时提醒已取消")
    } catch (reason) {
      notify.error(reason, { title: "取消临时提醒失败" })
    } finally {
      setRulePending(candidate.id, false)
    }
  }

  async function confirmExtend() {
    if (!extendCandidate) return
    const candidate = extendCandidate
    setRulePending(candidate.id, true)
    try {
      await updateRule(candidate.id, { duration: extendDuration })
      setExtendCandidate(null)
      notify.success("临时提醒已延长")
    } catch (reason) {
      notify.error(reason, { title: "延长临时提醒失败" })
    } finally {
      setRulePending(candidate.id, false)
    }
  }

  async function confirmDelete() {
    if (!deleteCandidate) return
    const candidate = deleteCandidate
    setRulePending(candidate.id, true)
    try {
      await deleteRule(candidate.id)
      setDeleteCandidate(null)
      notify.success("提醒记录已删除")
    } catch (reason) {
      notify.error(reason, { title: "删除提醒记录失败" })
    } finally {
      setRulePending(candidate.id, false)
    }
  }

  const schedulerUnavailable =
    overview &&
    (!overview.backgroundRemindersEnabled || !overview.accountRefreshEnabled)

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="right"
          className="data-[side=right]:w-[calc(100vw-0.5rem)] data-[side=right]:max-w-2xl data-[side=right]:overflow-y-auto data-[side=right]:p-0 sm:data-[side=right]:w-[min(42rem,calc(100vw-2rem))] sm:data-[side=right]:max-w-2xl! [&_[data-slot=sheet-close]]:z-20"
        >
          <SheetHeader className="sticky top-0 z-10 border-b bg-popover/95 py-4 pr-14 pl-5 backdrop-blur sm:pr-14 sm:pl-6">
            <SheetTitle>空闲提醒管理</SheetTitle>
            <SheetDescription>
              临时提醒会在发现空闲口、到期或取消后自动停止。
            </SheetDescription>
          </SheetHeader>

          <div className="flex flex-col gap-5 p-4 sm:p-6">
            {error ? (
              <Alert urgent variant="destructive" className="pr-24">
                <XCircleIcon />
                <AlertTitle>提醒设置暂时无法加载</AlertTitle>
                <AlertDescription>
                  请检查网络后重试，已有提醒不会因此被删除。
                  <details className="mt-1 text-xs">
                    <summary className="cursor-pointer">查看详情</summary>
                    <span className="break-words">{error}</span>
                  </details>
                </AlertDescription>
                <AlertAction>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void load()}
                  >
                    重试
                  </Button>
                </AlertAction>
              </Alert>
            ) : null}
            {loading && !loaded ? (
              <div className="grid gap-3 sm:grid-cols-3">
                <Skeleton className="h-24" />
                <Skeleton className="h-24" />
                <Skeleton className="h-24" />
              </div>
            ) : overview ? (
              <div className="grid grid-cols-2 gap-3">
                <Card size="sm">
                  <CardHeader>
                    <CardDescription>活动提醒充电桩</CardDescription>
                    <CardTitle className="text-xl tabular-nums">
                      {overview.reminderPileCount}/{overview.reminderPileLimit}
                    </CardTitle>
                  </CardHeader>
                </Card>
                <Card size="sm">
                  <CardHeader>
                    <CardDescription>今日已检查</CardDescription>
                    <CardTitle className="text-xl tabular-nums">
                      {overview.dailyQuotaUsed}/{overview.dailyQuotaLimit}
                    </CardTitle>
                  </CardHeader>
                </Card>
              </div>
            ) : null}

            {schedulerUnavailable ? (
              <Alert variant="destructive">
                <PowerOffIcon />
                <AlertTitle>后台提醒当前已暂停</AlertTitle>
                <AlertDescription>
                  {overview.accountRefreshEnabled
                    ? "管理员暂时关闭了空闲提醒。你的设置会继续保留。"
                    : "你的空闲提醒暂时不可用。设置恢复后会自动继续。"}
                </AlertDescription>
              </Alert>
            ) : null}

            {recurringRules.length > 0 &&
            !recurringRetirementNoticeDismissed ? (
              <Alert className="pr-24">
                <Clock3Icon />
                <AlertTitle>长期提醒已停用</AlertTitle>
                <AlertDescription>
                  旧设置已经停止后台检查。需要充电时，请重新开启临时提醒。
                </AlertDescription>
                <AlertAction>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={dismissRecurringRetirementNotice}
                  >
                    知道了
                  </Button>
                </AlertAction>
              </Alert>
            ) : null}

            <Tabs defaultValue="tasks">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="tasks">
                  <BellRingIcon data-icon="inline-start" />
                  空闲提醒
                </TabsTrigger>
                <TabsTrigger value="settings">
                  <Settings2Icon data-icon="inline-start" />
                  通知设置
                </TabsTrigger>
              </TabsList>

              <TabsContent value="tasks" className="flex flex-col gap-6 pt-3">
                <section
                  className="flex flex-col gap-3"
                  aria-labelledby="temporary-reminders-title"
                >
                  <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                      <h3
                        id="temporary-reminders-title"
                        className="font-medium"
                      >
                        临时提醒
                      </h3>
                      <p className="mt-1 text-xs text-muted-foreground">
                        需要充电时再开启，结束后不再查询。
                      </p>
                    </div>
                    <Button
                      size="sm"
                      className="w-full sm:w-auto"
                      disabled={piles.length === 0}
                      onClick={() => onEditRule({})}
                    >
                      <PlusIcon data-icon="inline-start" />
                      开始临时提醒
                    </Button>
                  </div>

                  {!loading &&
                  activeTemporaryRules.length === 0 &&
                  completedTemporaryRules.length === 0 ? (
                    <Empty className="min-h-52 border">
                      <EmptyHeader>
                        <EmptyMedia variant="icon">
                          <Clock3Icon />
                        </EmptyMedia>
                        <EmptyTitle>当前没有临时提醒</EmptyTitle>
                        <EmptyDescription>
                          选择一台充电桩，等待 1、2、4 小时或持续到今晚断电前。
                        </EmptyDescription>
                      </EmptyHeader>
                      <EmptyContent>
                        <Button
                          disabled={piles.length === 0}
                          onClick={() => onEditRule({})}
                        >
                          <BellRingIcon data-icon="inline-start" />
                          有空闲时提醒我
                        </Button>
                      </EmptyContent>
                    </Empty>
                  ) : null}

                  {activeTemporaryRules.map((rule) => {
                    const pending = pendingRuleIds.has(rule.id)
                    return (
                      <Card
                        key={rule.id}
                        size="sm"
                        className="motion-safe:animate-in motion-safe:duration-200 motion-safe:fade-in-0 motion-safe:slide-in-from-bottom-1"
                      >
                        <CardHeader>
                          <CardTitle>
                            {pileLabel(piles, rule.deviceId)}
                          </CardTitle>
                          <CardDescription>{ruleLabel()}</CardDescription>
                          <CardAction>
                            <Badge variant="default">
                              <Clock3Icon />
                              运行中
                            </Badge>
                          </CardAction>
                        </CardHeader>
                        <CardContent className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
                          <div className="rounded-lg bg-muted/60 p-3">
                            <p className="text-xs text-muted-foreground">
                              剩余时间
                            </p>
                            <p className="mt-1 font-medium tabular-nums">
                              {formatRemainingTime(rule.expiresAt, now)}
                            </p>
                          </div>
                          <div className="rounded-lg bg-muted/60 p-3">
                            <p className="text-xs text-muted-foreground">
                              下次检查
                            </p>
                            <p className="mt-1 font-medium tabular-nums">
                              {formatNextCheck(rule.nextCheckAt, now)}
                            </p>
                          </div>
                          <div className="col-span-2 rounded-lg bg-muted/60 p-3 sm:col-span-1">
                            <p className="text-xs text-muted-foreground">
                              预计还会检查
                            </p>
                            <p className="mt-1 font-medium tabular-nums">
                              {rule.estimatedRemainingChecks != null
                                ? `最多约 ${rule.estimatedRemainingChecks} 次`
                                : "等待下次检查"}
                            </p>
                          </div>
                        </CardContent>
                        <CardFooter className="justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={pending}
                            onClick={() => {
                              setExtendDuration("4h")
                              setExtendCandidate(rule)
                            }}
                          >
                            <RotateCwIcon data-icon="inline-start" />
                            延长
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={pending}
                            onClick={() => setCancelCandidate(rule)}
                          >
                            {pending ? (
                              <LoaderCircleIcon
                                data-icon="inline-start"
                                className="motion-safe:animate-spin"
                              />
                            ) : (
                              <XCircleIcon data-icon="inline-start" />
                            )}
                            取消提醒
                          </Button>
                        </CardFooter>
                      </Card>
                    )
                  })}

                  {completedTemporaryRules.length > 0 ? (
                    <div className="flex flex-col gap-3">
                      <p className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                        <HistoryIcon className="size-3.5" />
                        最近结束
                      </p>
                      {completedTemporaryRules.map((rule) => (
                        <Card key={rule.id} size="sm" className="bg-muted/20">
                          <CardHeader>
                            <CardTitle>
                              {pileLabel(piles, rule.deviceId)}
                            </CardTitle>
                            <CardDescription>
                              {formatCompletionReason(rule.completionReason)}
                            </CardDescription>
                            <CardAction>
                              <Badge variant="outline">
                                <CheckCircle2Icon />
                                已结束
                              </Badge>
                            </CardAction>
                          </CardHeader>
                          <CardContent>
                            <p className="text-xs text-muted-foreground">
                              结束于 {formatWatchTimestamp(rule.completedAt)}
                            </p>
                          </CardContent>
                          <CardFooter className="justify-end">
                            <Button
                              variant="ghost"
                              size="sm"
                              disabled={pendingRuleIds.has(rule.id)}
                              onClick={() => setDeleteCandidate(rule)}
                            >
                              <Trash2Icon data-icon="inline-start" />
                              清除记录
                            </Button>
                          </CardFooter>
                        </Card>
                      ))}
                    </div>
                  ) : null}
                </section>
              </TabsContent>

              <TabsContent
                value="settings"
                className="flex flex-col gap-4 pt-3"
              >
                <Card>
                  <CardHeader>
                    <CardTitle>免打扰时段</CardTitle>
                    <CardDescription>
                      在这段时间不弹出网页或微信提醒，消息仍会保留在通知中心。
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    {preference ? (
                      <QuietHoursForm
                        key={preference.updatedAt}
                        preference={preference}
                        onSave={updatePreference}
                      />
                    ) : (
                      <Skeleton className="h-44" />
                    )}
                  </CardContent>
                </Card>

                {overview ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>后台检查安排</CardTitle>
                      <CardDescription>
                        只会检查仍在等待空闲口的充电桩。
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="grid gap-3 sm:grid-cols-2">
                      <div className="flex items-center gap-3 rounded-lg border p-3">
                        <GaugeIcon className="size-5 text-muted-foreground" />
                        <div>
                          <p className="text-xs text-muted-foreground">
                            检查频率
                          </p>
                          <p className="mt-1 font-medium">
                            约每 {overview.refreshIntervalMinutes} 分钟一次
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-3 rounded-lg border p-3">
                        <PowerOffIcon className="size-5 text-muted-foreground" />
                        <div>
                          <p className="text-xs text-muted-foreground">
                            夜间暂停
                          </p>
                          <p className="mt-1 font-medium">
                            {overview.scheduledPowerOffEnabled
                              ? formatPowerWindow(
                                  overview.scheduledPowerOffStartMinute,
                                  overview.scheduledPowerOffEndMinute
                                )
                              : "未启用"}
                          </p>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ) : null}
              </TabsContent>
            </Tabs>
          </div>
        </SheetContent>
      </Sheet>

      <Dialog
        open={Boolean(extendCandidate)}
        onOpenChange={(next) => {
          if (!next) setExtendCandidate(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>延长临时提醒</DialogTitle>
            <DialogDescription>
              {overview?.scheduledPowerOffEnabled
                ? "从现在起重新计算等待时间，且不会超过今晚计划断电时间。"
                : "从现在起重新计算等待时间。"}
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="extend-duration">新的等待时间</FieldLabel>
            <Select
              items={availableDurationOptions.map((option) => ({
                value: option.value,
                label: option.label,
              }))}
              value={extendDuration}
              onValueChange={(value) =>
                value && setExtendDuration(value as WatchTemporaryDuration)
              }
            >
              <SelectTrigger id="extend-duration" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {availableDurationOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExtendCandidate(null)}>
              返回
            </Button>
            <Button onClick={() => void confirmExtend()}>
              <RotateCwIcon data-icon="inline-start" />
              确认延长
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(cancelCandidate)}
        onOpenChange={(next) => {
          if (!next) setCancelCandidate(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>取消这次临时提醒？</DialogTitle>
            <DialogDescription>
              取消后将立即停止后台检查，记录会保留在“最近结束”中。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCancelCandidate(null)}>
              继续等待
            </Button>
            <Button variant="destructive" onClick={() => void confirmCancel()}>
              <XCircleIcon data-icon="inline-start" />
              确认取消
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(deleteCandidate)}
        onOpenChange={(next) => {
          if (!next) setDeleteCandidate(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除这条提醒记录？</DialogTitle>
            <DialogDescription>
              {deleteCandidate
                ? `${pileLabel(piles, deleteCandidate.deviceId)} · ${ruleLabel()}，`
                : "这条提醒"}
              会从提醒列表移除，已收到的消息仍会保留在通知中心。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteCandidate(null)}>
              返回
            </Button>
            <Button variant="destructive" onClick={() => void confirmDelete()}>
              <Trash2Icon data-icon="inline-start" />
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
