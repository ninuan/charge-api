import type {
  WatchCompletionReason,
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

export function getTemporaryDurationOptions(scheduledPowerOffEnabled: boolean) {
  return scheduledPowerOffEnabled
    ? temporaryDurationOptions
    : temporaryDurationOptions.filter(
        (option) => option.value !== "until_power_off"
      )
}

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
  if (!nextCheckAt) return "等待下次检查"
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
