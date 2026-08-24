"use client"

import {
  BellRingIcon,
  CalendarClockIcon,
  CheckCircle2Icon,
  Clock3Icon,
  LoaderCircleIcon,
} from "lucide-react"
import { useMemo, useState } from "react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
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
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type {
  WatchRule,
  WatchRuleCreateResult,
  WatchRuleMode,
  WatchTemporaryDuration,
} from "@/lib/api/generated"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import {
  estimatedTemporaryChecks,
  estimatedRecurringChecks,
  formatActiveTime,
  minutesToTime,
  temporaryDurationOptions,
  timeToMinutes,
  weekdays,
} from "@/lib/watch-format"

export type WatchEditorTarget = {
  pileId?: string
  ruleId?: string
  mode?: WatchRuleMode
}

type FormState = {
  pileId: string
  mode: WatchRuleMode
  duration: WatchTemporaryDuration
  enabled: boolean
  activeWeekdays: number
  start: string
  end: string
}

function initialForm(
  piles: Pile[],
  target: WatchEditorTarget,
  rule?: WatchRule
): FormState {
  return {
    pileId: rule?.deviceId ?? target.pileId ?? piles[0]?.id ?? "",
    mode: rule?.mode ?? target.mode ?? "temporary",
    duration: "2h",
    enabled: rule?.enabled ?? true,
    activeWeekdays: rule?.activeWeekdays ?? 127,
    start: minutesToTime(rule?.activeStartMinute ?? 0),
    end: minutesToTime(rule?.activeEndMinute ?? 0),
  }
}

function localMinute(timezone: string) {
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: timezone,
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).formatToParts(new Date())
  const hour = Number(parts.find((part) => part.type === "hour")?.value ?? 0)
  const minute = Number(
    parts.find((part) => part.type === "minute")?.value ?? 0
  )
  return (hour % 24) * 60 + minute
}

function isWithinWindow(value: number, start: number, end: number) {
  if (start === end) return true
  return start < end
    ? value >= start && value < end
    : value >= start || value < end
}

export function WatchRuleDialog({
  piles,
  target,
  open,
  onOpenChange,
}: {
  piles: Pile[]
  target: WatchEditorTarget
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { rules, overview, createRule, updateRule } = useWatch()
  const rule = useMemo(
    () => rules.find((candidate) => candidate.id === target.ruleId),
    [rules, target.ruleId]
  )
  const [form, setForm] = useState<FormState>(() =>
    initialForm(piles, target, rule)
  )
  const [saving, setSaving] = useState(false)
  const [createResult, setCreateResult] =
    useState<WatchRuleCreateResult | null>(null)
  const startMinute = timeToMinutes(form.start)
  const endMinute = timeToMinutes(form.end)
  const pileOptions = useMemo(
    () =>
      piles.map((pile) => ({
        value: pile.id,
        label: pile.name || pile.number || pile.id,
      })),
    [piles]
  )
  const durationOptions = useMemo(
    () =>
      temporaryDurationOptions.map((option) => ({
        value: option.value,
        label: option.label,
      })),
    []
  )
  const estimatedChecks = overview
    ? estimatedTemporaryChecks(form.duration, overview.refreshIntervalMinutes)
    : null
  const estimatedRecurringChecksPerDay =
    overview && startMinute != null && endMinute != null
      ? estimatedRecurringChecks(
          startMinute,
          endMinute,
          overview.refreshIntervalMinutes
        )
      : null
  const powerOffActive = Boolean(
    overview?.scheduledPowerOffEnabled &&
    isWithinWindow(
      localMinute(overview.scheduledPowerOffTimezone),
      overview.scheduledPowerOffStartMinute,
      overview.scheduledPowerOffEndMinute
    )
  )
  const recurringInvalid =
    form.activeWeekdays === 0 || startMinute == null || endMinute == null
  const invalid =
    !form.pileId ||
    (form.mode === "temporary" && powerOffActive) ||
    (form.mode === "recurring" && recurringInvalid)

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (invalid) return
    setSaving(true)
    try {
      if (rule) {
        if (startMinute == null || endMinute == null) return
        await updateRule(rule.id, {
          enabled: form.enabled,
          activeWeekdays: form.activeWeekdays,
          activeStartMinute: startMinute,
          activeEndMinute: endMinute,
          timezone: "Asia/Shanghai",
        })
        toast.success("固定时段提醒已更新")
        onOpenChange(false)
        return
      }

      if (form.mode === "temporary") {
        const result = await createRule({
          deviceId: form.pileId,
          duration: form.duration,
        })
        if (!result.backgroundScheduled && result.idlePortIds.length > 0) {
          setCreateResult(result)
          toast.success("现在就有空闲充电口")
          return
        }
        toast.success(result.message)
        onOpenChange(false)
        return
      }

      if (startMinute == null || endMinute == null) return
      const result = await createRule({
        deviceId: form.pileId,
        mode: "recurring",
        enabled: form.enabled,
        activeWeekdays: form.activeWeekdays,
        activeStartMinute: startMinute,
        activeEndMinute: endMinute,
        timezone: "Asia/Shanghai",
      })
      toast.success(result.message)
      onOpenChange(false)
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (createResult) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>现在就有空闲充电口</DialogTitle>
            <DialogDescription>
              已完成本次查询，不会创建后台提醒任务。
            </DialogDescription>
          </DialogHeader>
          <Alert className="border-success/35 bg-success/8 text-success">
            <CheckCircle2Icon />
            <AlertTitle>可用端口</AlertTitle>
            <AlertDescription className="text-foreground">
              {createResult.idlePortIds
                .map((portId) => `${portId} 号口`)
                .join("、")}
            </AlertDescription>
          </Alert>
          <DialogFooter>
            <Button onClick={() => onOpenChange(false)}>知道了</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  const recurringFields = (
    <>
      <FieldSet>
        <FieldLegend>生效星期</FieldLegend>
        <div className="grid grid-cols-4 gap-2 sm:grid-cols-7">
          {weekdays.map((weekday) => {
            const checked = (form.activeWeekdays & weekday.bit) !== 0
            return (
              <FieldLabel key={weekday.bit} className="justify-center">
                <Checkbox
                  aria-label={weekday.label}
                  checked={checked}
                  onCheckedChange={(nextChecked) =>
                    setForm((current) => ({
                      ...current,
                      activeWeekdays: nextChecked
                        ? current.activeWeekdays | weekday.bit
                        : current.activeWeekdays & ~weekday.bit,
                    }))
                  }
                />
                周{weekday.short}
              </FieldLabel>
            )
          })}
        </div>
        {form.activeWeekdays === 0 ? (
          <p className="text-sm text-destructive" role="alert">
            至少选择一天
          </p>
        ) : null}
      </FieldSet>

      <FieldGroup className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="watch-start">开始时间</FieldLabel>
          <Input
            id="watch-start"
            type="time"
            value={form.start}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                start: event.target.value,
              }))
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="watch-end">结束时间</FieldLabel>
          <Input
            id="watch-end"
            type="time"
            value={form.end}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                end: event.target.value,
              }))
            }
          />
        </Field>
      </FieldGroup>
      <p className="rounded-lg bg-muted px-3 py-2 text-xs leading-5 text-muted-foreground">
        当前时段：
        {startMinute != null && endMinute != null
          ? formatActiveTime(startMinute, endMinute)
          : "请选择有效时间"}
        。开始与结束相同表示全天；结束时间早于开始时间时，提醒会持续到第二天。
      </p>

      <Field orientation="horizontal">
        <FieldContent>
          <FieldLabel htmlFor="watch-enabled">启用固定提醒</FieldLabel>
          <FieldDescription>
            关闭后会保存配置，但不会进行后台检查。
          </FieldDescription>
        </FieldContent>
        <Switch
          id="watch-enabled"
          checked={form.enabled}
          onCheckedChange={(checked) =>
            setForm((current) => ({ ...current, enabled: checked }))
          }
        />
      </Field>
    </>
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {rule
              ? "编辑固定时段提醒"
              : form.mode === "recurring"
                ? "设置固定时段提醒"
                : "有空闲时提醒我"}
          </DialogTitle>
          <DialogDescription>
            {rule
              ? "调整这条长期运行的固定时段提醒。"
              : form.mode === "recurring"
                ? "设置每周重复生效的星期和时间，适合长期固定需求。"
                : "先查看一次当前状态；没有空闲口时，再在有限时间内低频检查。"}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit}>
          <FieldGroup>
            <Field data-disabled={Boolean(rule)}>
              <FieldLabel htmlFor="watch-pile">充电桩</FieldLabel>
              <Select
                items={pileOptions}
                value={form.pileId}
                disabled={Boolean(rule)}
                onValueChange={(value) =>
                  value &&
                  setForm((current) => ({
                    ...current,
                    pileId: value,
                  }))
                }
              >
                <SelectTrigger id="watch-pile" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {pileOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>

            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel>
                  <BellRingIcon className="size-4" />
                  提醒范围
                </FieldLabel>
                <FieldDescription>
                  整台充电桩。任意一个充电口空闲就提醒你。
                </FieldDescription>
              </FieldContent>
            </Field>

            {rule ? (
              recurringFields
            ) : (
              <Tabs
                value={form.mode}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    mode: value as WatchRuleMode,
                  }))
                }
              >
                <TabsList className="grid w-full grid-cols-2">
                  <TabsTrigger value="temporary">
                    <Clock3Icon data-icon="inline-start" />
                    临时提醒
                  </TabsTrigger>
                  <TabsTrigger value="recurring">
                    <CalendarClockIcon data-icon="inline-start" />
                    固定时段（高级）
                  </TabsTrigger>
                </TabsList>

                <TabsContent
                  value="temporary"
                  className="flex flex-col gap-4 pt-3"
                >
                  <Field>
                    <FieldLabel htmlFor="watch-duration">等待多久</FieldLabel>
                    <Select
                      items={durationOptions}
                      value={form.duration}
                      onValueChange={(value) =>
                        value &&
                        setForm((current) => ({
                          ...current,
                          duration: value as WatchTemporaryDuration,
                        }))
                      }
                    >
                      <SelectTrigger id="watch-duration" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {temporaryDurationOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FieldDescription>
                      {estimatedChecks
                        ? `没有空闲口时，后台最多约检查 ${estimatedChecks} 次。`
                        : "任务会在今晚断电前自动结束。"}
                      发现空闲后会提醒一次并自动停止。
                    </FieldDescription>
                  </Field>

                  {powerOffActive && overview ? (
                    <Alert variant="destructive">
                      <Clock3Icon />
                      <AlertTitle>当前处于夜间停电时段</AlertTitle>
                      <AlertDescription>
                        暂时不能开始提醒，预计{" "}
                        {minutesToTime(overview.scheduledPowerOffEndMinute)}{" "}
                        恢复供电后可用。
                      </AlertDescription>
                    </Alert>
                  ) : (
                    <Alert>
                      <Clock3Icon />
                      <AlertTitle>这是一项临时任务</AlertTitle>
                      <AlertDescription>
                        实际结束时间不会晚于今晚计划断电时间，你也可以随时取消或延长。
                      </AlertDescription>
                    </Alert>
                  )}
                </TabsContent>

                <TabsContent
                  value="recurring"
                  className="flex flex-col gap-4 pt-3"
                >
                  <Alert>
                    <CalendarClockIcon />
                    <AlertTitle>固定时段提醒会长期运行</AlertTitle>
                    <AlertDescription>
                      它会在每个选定时段重复检查，适合长期固定需求，也会产生更多后台查询。
                      {estimatedRecurringChecksPerDay
                        ? ` 每个生效日最多约检查 ${estimatedRecurringChecksPerDay} 次。`
                        : ""}
                    </AlertDescription>
                  </Alert>
                  {recurringFields}
                </TabsContent>
              </Tabs>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={saving || invalid}>
                {saving ? (
                  <LoaderCircleIcon
                    data-icon="inline-start"
                    className="animate-spin"
                  />
                ) : null}
                {saving
                  ? "保存中…"
                  : rule
                    ? "保存修改"
                    : form.mode === "temporary"
                      ? "开始提醒"
                      : "创建固定提醒"}
              </Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  )
}
