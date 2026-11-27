import { describe, expect, it } from "vitest"

import {
  estimatedTemporaryChecks,
  formatCompletionReason,
  formatNextCheck,
  formatPowerWindow,
  formatRemainingTime,
  getTemporaryDurationOptions,
  minutesToTime,
  timeToMinutes,
} from "@/lib/watch-format"

describe("watch formatting", () => {
  it("formats power-off windows explicitly", () => {
    expect(formatPowerWindow(23 * 60, 7 * 60)).toBe("23:00–07:00")
  })

  it("round-trips valid times and rejects invalid values", () => {
    expect(minutesToTime(419)).toBe("06:59")
    expect(timeToMinutes("06:59")).toBe(419)
    expect(timeToMinutes("24:00")).toBeNull()
    expect(timeToMinutes("7:00")).toBeNull()
  })

  it("formats temporary reminder timing and completion states", () => {
    const now = new Date("2026-08-10T10:00:00Z").getTime()
    expect(estimatedTemporaryChecks("2h", 10)).toBe(12)
    expect(estimatedTemporaryChecks("until_power_off", 10)).toBeNull()
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

  it("only offers the power-off duration when scheduled power-off is enabled", () => {
    expect(
      getTemporaryDurationOptions(false).map((option) => option.value)
    ).toEqual(["1h", "2h", "4h"])
    expect(
      getTemporaryDurationOptions(true).map((option) => option.value)
    ).toContain("until_power_off")
  })
})
