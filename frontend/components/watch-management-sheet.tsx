"use client"

import {
  BellRingIcon,
  CalendarClockIcon,
  GaugeIcon,
  LoaderCircleIcon,
  MoonIcon,
  PencilIcon,
  PlusIcon,
  PowerOffIcon,
  Settings2Icon,
  Trash2Icon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import type { WatchEditorTarget } from "@/components/watch-rule-dialog"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
} from "@/lib/api/generated"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import {
  formatPowerWindow,
  formatRuleSchedule,
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

function QuietHoursForm({
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
      toast.error("请选择有效的免打扰时段")
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
      toast.success("免打扰设置已保存")
    } catch (reason) {
      toast.error((reason as Error).message)
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
              开启后，这段时间内不会弹出浏览器提醒。
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
              className="animate-spin"
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
    load,
    updateRule,
    deleteRule,
    updatePreference,
  } = useWatch()
  const [pendingRuleIds, setPendingRuleIds] = useState<Set<string>>(
    () => new Set()
  )
  const [deleteCandidate, setDeleteCandidate] = useState<WatchRule | null>(null)

  useEffect(() => {
    if (open && !loaded && !loading) {
      void load().catch((reason) => toast.error((reason as Error).message))
    }
  }, [load, loaded, loading, open])

  const orderedRules = useMemo(
    () =>
      rules.toSorted((left, right) => {
        const pileCompare = pileLabel(piles, left.deviceId).localeCompare(
          pileLabel(piles, right.deviceId),
          "zh-CN"
        )
        return pileCompare
      }),
    [piles, rules]
  )

  function setRulePending(ruleId: string, pending: boolean) {
    setPendingRuleIds((current) => {
      const next = new Set(current)
      if (pending) next.add(ruleId)
      else next.delete(ruleId)
      return next
    })
  }

  async function toggleRule(rule: WatchRule, enabled: boolean) {
    setRulePending(rule.id, true)
    try {
      await updateRule(rule.id, { enabled })
      toast.success(enabled ? "空闲提醒已启用" : "空闲提醒已停用")
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setRulePending(rule.id, false)
    }
  }

  async function confirmDelete() {
    if (!deleteCandidate) return
    const candidate = deleteCandidate
    setRulePending(candidate.id, true)
    try {
      await deleteRule(candidate.id)
      setDeleteCandidate(null)
      toast.success("空闲提醒已删除")
    } catch (reason) {
      toast.error((reason as Error).message)
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
          className="w-[calc(100vw-0.5rem)] max-w-2xl overflow-y-auto p-0 sm:w-[min(42rem,calc(100vw-2rem))] sm:max-w-2xl [&_[data-slot=sheet-close]]:z-20"
        >
          <SheetHeader className="sticky top-0 z-10 border-b bg-popover/95 py-4 pr-14 pl-5 backdrop-blur sm:pr-14 sm:pl-6">
            <SheetTitle>空闲提醒管理</SheetTitle>
            <SheetDescription>
              选择充电桩和提醒时段，有空闲充电口时通知你。
            </SheetDescription>
          </SheetHeader>

          <div className="flex flex-col gap-5 p-4 sm:p-6">
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
                    <CardDescription>提醒充电桩</CardDescription>
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

            <Tabs defaultValue="rules">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="rules">
                  <BellRingIcon data-icon="inline-start" />
                  提醒列表
                </TabsTrigger>
                <TabsTrigger value="settings">
                  <Settings2Icon data-icon="inline-start" />
                  提醒设置
                </TabsTrigger>
              </TabsList>

              <TabsContent value="rules" className="flex flex-col gap-3 pt-3">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h3 className="font-medium">已设置提醒的充电桩</h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      暂时停用不会丢失已经选择的时段。
                    </p>
                  </div>
                  <Button
                    size="sm"
                    disabled={piles.length === 0}
                    onClick={() => onEditRule({})}
                  >
                    <PlusIcon data-icon="inline-start" />
                    添加提醒
                  </Button>
                </div>

                {!loading && orderedRules.length === 0 ? (
                  <Empty className="min-h-60 border">
                    <EmptyHeader>
                      <EmptyMedia variant="icon">
                        <BellRingIcon />
                      </EmptyMedia>
                      <EmptyTitle>还没有空闲提醒</EmptyTitle>
                      <EmptyDescription>
                        选择一台充电桩，设置希望收到提醒的星期和时间。
                      </EmptyDescription>
                    </EmptyHeader>
                    <EmptyContent>
                      <Button
                        disabled={piles.length === 0}
                        onClick={() => onEditRule({})}
                      >
                        <PlusIcon data-icon="inline-start" />
                        添加第一个提醒
                      </Button>
                    </EmptyContent>
                  </Empty>
                ) : (
                  orderedRules.map((rule) => {
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
                            <Switch
                              size="sm"
                              aria-label={
                                rule.enabled
                                  ? "停用空闲提醒"
                                  : "启用空闲提醒"
                              }
                              checked={rule.enabled}
                              disabled={pending}
                              onCheckedChange={(enabled) =>
                                void toggleRule(rule, enabled)
                              }
                            />
                          </CardAction>
                        </CardHeader>
                        <CardContent className="flex flex-wrap items-center gap-2">
                          <Badge variant="default">
                            <BellRingIcon />
                            整桩空闲提醒
                          </Badge>
                          <Badge
                            variant={rule.enabled ? "secondary" : "outline"}
                          >
                            {rule.enabled ? "已启用" : "已停用"}
                          </Badge>
                          <span className="flex items-center gap-1 text-xs text-muted-foreground">
                            <CalendarClockIcon className="size-3.5" />
                            {formatRuleSchedule(rule)}
                          </span>
                        </CardContent>
                        <CardFooter className="justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={pending}
                            onClick={() => onEditRule({ ruleId: rule.id })}
                          >
                            <PencilIcon data-icon="inline-start" />
                            编辑
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={pending}
                            onClick={() => setDeleteCandidate(rule)}
                          >
                            {pending ? (
                              <LoaderCircleIcon
                                data-icon="inline-start"
                                className="animate-spin"
                              />
                            ) : (
                              <Trash2Icon data-icon="inline-start" />
                            )}
                            删除
                          </Button>
                        </CardFooter>
                      </Card>
                    )
                  })
                )}
              </TabsContent>

              <TabsContent
                value="settings"
                className="flex flex-col gap-4 pt-3"
              >
                <Card>
                  <CardHeader>
                    <CardTitle>免打扰时段</CardTitle>
                    <CardDescription>
                      在这段时间不弹出浏览器提醒，通知仍会保留在通知中心。
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
                      <CardTitle>提醒时间</CardTitle>
                      <CardDescription>
                        系统会根据你的提醒设置查看空闲情况。
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
        open={Boolean(deleteCandidate)}
        onOpenChange={(next) => {
          if (!next) setDeleteCandidate(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除这条空闲提醒？</DialogTitle>
            <DialogDescription>
              {deleteCandidate
                ? `${pileLabel(piles, deleteCandidate.deviceId)} · ${ruleLabel()}，`
                : "这条提醒"}
              将从提醒列表移除，已有站内通知不会删除。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteCandidate(null)}>
              取消
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
