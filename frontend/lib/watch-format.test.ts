import { describe, expect, it } from "vitest"

import {
  estimatedTemporaryChecks,
  estimatedRecurringChecks,
  formatActiveTime,
  formatCompletionReason,
  formatNextCheck,
  formatPowerWindow,
  formatRemainingTime,
  formatWeekdays,
  minutesToTime,
  timeToMinutes,
} from "@/lib/watch-format"

describe("watch formatting", () => {
  it("formats all-day, daytime, and cross-midnight windows explicitly", () => {
    expect(formatActiveTime(0, 0)).toBe("全天")
    expect(formatActiveTime(8 * 60, 18 * 60)).toBe("08:00–18:00")
    expect(formatActiveTime(22 * 60, 7 * 60)).toBe("22:00–07:00（跨午夜）")
    expect(formatPowerWindow(23 * 60, 7 * 60)).toBe("23:00–07:00")
  })

  it("round-trips valid times and rejects invalid values", () => {
    expect(minutesToTime(419)).toBe("06:59")
    expect(timeToMinutes("06:59")).toBe(419)
    expect(timeToMinutes("24:00")).toBeNull()
    expect(timeToMinutes("7:00")).toBeNull()
  })

  it("uses compact labels for common weekday masks", () => {
    expect(formatWeekdays(127)).toBe("每天")
    expect(formatWeekdays(31)).toBe("工作日")
    expect(formatWeekdays(96)).toBe("周末")
    expect(formatWeekdays(65)).toBe("周一、周日")
  })

  it("formats temporary reminder timing and completion states", () => {
    const now = new Date("2026-08-10T10:00:00Z").getTime()
    expect(estimatedTemporaryChecks("2h", 10)).toBe(12)
    expect(estimatedTemporaryChecks("until_power_off", 10)).toBeNull()
    expect(estimatedRecurringChecks(22 * 60, 7 * 60, 10)).toBe(54)
    expect(estimatedRecurringChecks(0, 0, 10)).toBe(144)
    expect(formatRemainingTime("2026-08-10T11:25:00Z", now)).toBe(
      "1 小时 25 分钟"
    )
    expect(formatNextCheck("2026-08-10T10:10:00Z", now)).toContain(
      "约 10 分钟后"
    )
    expect(formatCompletionReason("notified")).toBe("已发现空闲口并完成提醒")
    expect(formatCompletionReason("expired")).toBe("等待时间已结束")
    expect(formatCompletionReason("cancelled")).toBe("已由你取消")
  })
})
