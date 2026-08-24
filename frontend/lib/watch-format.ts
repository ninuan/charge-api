import type {
  WatchCompletionReason,
  WatchRule,
  WatchTemporaryDuration,
} from "@/lib/api/generated"

export const temporaryDurationOptions: ReadonlyArray<{
  value: WatchTemporaryDuration
  label: string
  shortLabel: string
  minutes?: number
}> = [
  { value: "1h", label: "1 小时", shortLabel: "1 小时", minutes: 60 },
  { value: "2h", label: "2 小时（推荐）", shortLabel: "2 小时", minutes: 120 },
  { value: "4h", label: "4 小时", shortLabel: "4 小时", minutes: 240 },
  {
    value: "until_power_off",
    label: "持续到今晚断电前",
    shortLabel: "到断电前",
  },
]

export const weekdays = [
  { bit: 1, short: "一", label: "周一" },
  { bit: 2, short: "二", label: "周二" },
  { bit: 4, short: "三", label: "周三" },
  { bit: 8, short: "四", label: "周四" },
  { bit: 16, short: "五", label: "周五" },
  { bit: 32, short: "六", label: "周六" },
  { bit: 64, short: "日", label: "周日" },
] as const

export function minutesToTime(value: number) {
  const normalized = Math.max(0, Math.min(1439, value))
  return `${String(Math.floor(normalized / 60)).padStart(2, "0")}:${String(normalized % 60).padStart(2, "0")}`
}

export function timeToMinutes(value: string) {
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  if (hour > 23 || minute > 59) return null
  return hour * 60 + minute
}

export function formatWeekdays(mask: number) {
  if (mask === 127) return "每天"
  if (mask === 31) return "工作日"
  if (mask === 96) return "周末"
  return weekdays
    .filter((weekday) => (mask & weekday.bit) !== 0)
    .map((weekday) => weekday.label)
    .join("、")
}

export function formatActiveTime(start: number, end: number) {
  if (start === end) return "全天"
  const suffix = start > end ? "（跨午夜）" : ""
  return `${minutesToTime(start)}–${minutesToTime(end)}${suffix}`
}

export function formatRuleSchedule(rule: WatchRule) {
  return `${formatWeekdays(rule.activeWeekdays)} · ${formatActiveTime(rule.activeStartMinute, rule.activeEndMinute)}`
}

export function formatPowerWindow(start: number, end: number) {
  return `${minutesToTime(start)}–${minutesToTime(end)}`
}

export function estimatedTemporaryChecks(
  duration: WatchTemporaryDuration,
  intervalMinutes: number
) {
  const durationMinutes = temporaryDurationOptions.find(
    (option) => option.value === duration
  )?.minutes
  if (!durationMinutes || intervalMinutes <= 0) return null
  return Math.max(1, Math.ceil(durationMinutes / intervalMinutes))
}

export function estimatedRecurringChecks(
  startMinute: number,
  endMinute: number,
  intervalMinutes: number
) {
  if (intervalMinutes <= 0) return null
  const durationMinutes =
    startMinute === endMinute
      ? 24 * 60
      : endMinute > startMinute
        ? endMinute - startMinute
        : 24 * 60 - startMinute + endMinute
  return Math.max(1, Math.ceil(durationMinutes / intervalMinutes))
}

export function formatRemainingTime(expiresAt?: string, now = Date.now()) {
  if (!expiresAt) return "等待结束时间"
  const remainingMinutes = Math.max(
    0,
    Math.ceil((new Date(expiresAt).getTime() - now) / 60_000)
  )
  if (remainingMinutes <= 0) return "即将结束"
  if (remainingMinutes < 60) return `约 ${remainingMinutes} 分钟`
  const hours = Math.floor(remainingMinutes / 60)
  const minutes = remainingMinutes % 60
  return minutes > 0 ? `${hours} 小时 ${minutes} 分钟` : `${hours} 小时`
}

export function formatNextCheck(nextCheckAt?: string, now = Date.now()) {
  if (!nextCheckAt) return "等待调度"
  const next = new Date(nextCheckAt)
  const minutes = Math.ceil((next.getTime() - now) / 60_000)
  if (minutes <= 0) return "即将检查"
  const time = next.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  })
  return `${time}（约 ${minutes} 分钟后）`
}

export function formatWatchTimestamp(value?: string) {
  if (!value) return "--"
  return new Date(value).toLocaleString("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  })
}

export function formatCompletionReason(reason?: WatchCompletionReason) {
  switch (reason) {
    case "notified":
      return "已发现空闲口并完成提醒"
    case "expired":
      return "等待时间已结束"
    case "cancelled":
      return "已由你取消"
    default:
      return "提醒已结束"
  }
}
