import type { WatchRule } from "@/lib/api/generated"

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
