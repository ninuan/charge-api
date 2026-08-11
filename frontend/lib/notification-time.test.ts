import { describe, expect, it } from "vitest"

import type { NotificationPreference } from "@/lib/api/generated"
import { isQuietHours } from "@/lib/notification-time"

const preference: NotificationPreference = {
  userId: "user-1",
  browserEnabled: true,
  quietHoursEnabled: true,
  quietStartMinute: 22 * 60,
  quietEndMinute: 8 * 60,
  timezone: "Asia/Shanghai",
  updatedAt: "2026-08-11T00:00:00Z",
}

describe("notification quiet hours", () => {
  it("handles a cross-midnight quiet window in the configured timezone", () => {
    expect(isQuietHours(preference, new Date("2026-08-11T15:30:00Z"))).toBe(
      true
    )
    expect(isQuietHours(preference, new Date("2026-08-11T00:30:00Z"))).toBe(
      false
    )
  })

  it("does not suppress browser delivery when quiet hours are disabled", () => {
    expect(
      isQuietHours(
        { ...preference, quietHoursEnabled: false },
        new Date("2026-08-11T15:30:00Z")
      )
    ).toBe(false)
  })
})
