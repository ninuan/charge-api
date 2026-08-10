import { describe, expect, it } from "vitest"

import {
  formatActiveTime,
  formatPowerWindow,
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
})
