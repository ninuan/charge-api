"use client"

import { BellRingIcon, LoaderCircleIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { toast } from "sonner"

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
import type { WatchRule } from "@/lib/api/generated"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import {
  formatActiveTime,
  minutesToTime,
  timeToMinutes,
  weekdays,
} from "@/lib/watch-format"

export type WatchEditorTarget = {
  pileId?: string
  ruleId?: string
}

type FormState = {
  pileId: string
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
    enabled: rule?.enabled ?? true,
    activeWeekdays: rule?.activeWeekdays ?? 127,
    start: minutesToTime(rule?.activeStartMinute ?? 0),
    end: minutesToTime(rule?.activeEndMinute ?? 0),
  }
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
  const { rules, createRule, updateRule } = useWatch()
  const rule = useMemo(
    () => rules.find((candidate) => candidate.id === target.ruleId),
    [rules, target.ruleId]
  )
  const [form, setForm] = useState<FormState>(() =>
    initialForm(piles, target, rule)
  )
  const [saving, setSaving] = useState(false)
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

  const invalid =
    !form.pileId ||
    form.activeWeekdays === 0 ||
    startMinute == null ||
    endMinute == null

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (invalid || startMinute == null || endMinute == null) return
    setSaving(true)
    try {
      if (rule) {
        await updateRule(rule.id, {
          enabled: form.enabled,
          activeWeekdays: form.activeWeekdays,
          activeStartMinute: startMinute,
          activeEndMinute: endMinute,
          timezone: "Asia/Shanghai",
        })
        toast.success("空闲提醒已更新")
      } else {
        await createRule({
          deviceId: form.pileId,
          mode: "recurring",
          enabled: form.enabled,
          activeWeekdays: form.activeWeekdays,
          activeStartMinute: startMinute,
          activeEndMinute: endMinute,
          timezone: "Asia/Shanghai",
        })
        toast.success("整桩空闲提醒已创建")
      }
      onOpenChange(false)
    } catch (reason) {
      toast.error((reason as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{rule ? "编辑空闲提醒" : "添加空闲提醒"}</DialogTitle>
          <DialogDescription>
            选择你需要充电的时段，有空闲充电口时通知你。
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit}>
          <FieldGroup>
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
            </FieldGroup>

            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel>
                  <BellRingIcon className="size-4" />
                  提醒范围
                </FieldLabel>
                <FieldDescription>
                  整台充电桩。任意充电口从使用中变为空闲时提醒一次。
                </FieldDescription>
              </FieldContent>
            </Field>

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
                <FieldLabel htmlFor="watch-enabled">启用提醒</FieldLabel>
                <FieldDescription>
                  关闭后不再接收提醒，已经选择的时段会保留。
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
                {saving ? "保存中…" : rule ? "保存修改" : "创建提醒"}
              </Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  )
}
