import type { NotificationPreference } from "@/lib/api/generated"

function minuteInTimezone(date: Date, timezone: string) {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(date)
  const hour = Number(parts.find((part) => part.type === "hour")?.value ?? 0)
  const minute = Number(
    parts.find((part) => part.type === "minute")?.value ?? 0
  )
  return hour * 60 + minute
}

export function isQuietHours(
  preference: NotificationPreference | null,
  date = new Date()
) {
  if (!preference?.quietHoursEnabled) return false
  const current = minuteInTimezone(date, preference.timezone)
  const { quietStartMinute: start, quietEndMinute: end } = preference
  if (start === end) return true
  return start < end
    ? current >= start && current < end
    : current >= start || current < end
}
