"use client"

import {
  BellRingIcon,
  CheckCircle2Icon,
  Clock3Icon,
  LoaderCircleIcon,
} from "lucide-react"
import { useMemo, useState } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
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
} from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type {
  WatchRuleCreateResult,
  WatchTemporaryDuration,
} from "@/lib/api/generated"
import { notify } from "@/lib/feedback"
import type { Pile } from "@/lib/types"
import { useWatch } from "@/lib/watch-context"
import {
  estimatedTemporaryChecks,
  getTemporaryDurationOptions,
  minutesToTime,
} from "@/lib/watch-format"

export type WatchEditorTarget = {
  pileId?: string
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
  const { overview, createRule } = useWatch()
  const [pileId, setPileId] = useState(
    () => target.pileId ?? piles[0]?.id ?? ""
  )
  const [duration, setDuration] = useState<WatchTemporaryDuration>("2h")
  const [saving, setSaving] = useState(false)
  const [createResult, setCreateResult] =
    useState<WatchRuleCreateResult | null>(null)
  const pileOptions = useMemo(
    () =>
      piles.map((pile) => ({
        value: pile.id,
        label: pile.name || pile.number || pile.id,
      })),
    [piles]
  )
  const availableDurationOptions = useMemo(
    () =>
      getTemporaryDurationOptions(Boolean(overview?.scheduledPowerOffEnabled)),
    [overview?.scheduledPowerOffEnabled]
  )
  const durationOptions = useMemo(
    () =>
      availableDurationOptions.map((option) => ({
        value: option.value,
        label: option.label,
      })),
    [availableDurationOptions]
  )
  const estimatedChecks = overview
    ? estimatedTemporaryChecks(duration, overview.refreshIntervalMinutes)
    : null
  const powerOffActive = Boolean(
    overview?.scheduledPowerOffEnabled &&
    isWithinWindow(
      localMinute(overview.scheduledPowerOffTimezone),
      overview.scheduledPowerOffStartMinute,
      overview.scheduledPowerOffEndMinute
    )
  )

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!pileId || powerOffActive) return
    setSaving(true)
    try {
      const result = await createRule({ deviceId: pileId, duration })
      if (!result.backgroundScheduled && result.idlePortIds.length > 0) {
        setCreateResult(result)
        return
      }
      notify.success("空闲提醒已开启", { description: result.message })
      onOpenChange(false)
    } catch (reason) {
      notify.error(reason, { title: "开启空闲提醒失败" })
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
              已完成本次查询，不会创建后台提醒。
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>有空闲时提醒我</DialogTitle>
          <DialogDescription>
            先查看当前状态；没有空闲口时，在你选择的时间内继续检查。
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="watch-pile">充电桩</FieldLabel>
              <Select
                items={pileOptions}
                value={pileId}
                onValueChange={(value) => value && setPileId(value)}
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

            <Field>
              <FieldLabel htmlFor="watch-duration">等待多久</FieldLabel>
              <Select
                items={durationOptions}
                value={duration}
                onValueChange={(value) =>
                  value && setDuration(value as WatchTemporaryDuration)
                }
              >
                <SelectTrigger id="watch-duration" className="w-full">
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
              <FieldDescription>
                {estimatedChecks
                  ? `没有空闲口时，最多约检查 ${estimatedChecks} 次。`
                  : "提醒会在今晚断电前自动结束。"}
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
                <AlertTitle>本次提醒会自动结束</AlertTitle>
                <AlertDescription>
                  发现空闲口、到达等待时间或进入计划断电时段后停止。
                </AlertDescription>
              </Alert>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={saving || !pileId || powerOffActive}
              >
                {saving ? (
                  <LoaderCircleIcon
                    data-icon="inline-start"
                    className="motion-safe:animate-spin"
                  />
                ) : null}
                {saving ? "开启中…" : "开始提醒"}
              </Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  )
}
