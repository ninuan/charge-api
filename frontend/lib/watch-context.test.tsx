import { act, cleanup, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import type {
  NotificationPreference,
  WatchOverview,
  WatchRule,
} from "@/lib/api/generated"
import { WatchProvider, useWatch } from "@/lib/watch-context"

const { watchApiMock } = vi.hoisted(() => ({
  watchApiMock: {
    rules: vi.fn(),
    overview: vi.fn(),
    preference: vi.fn(),
    createRule: vi.fn(),
    updateRule: vi.fn(),
    deleteRule: vi.fn(),
    updatePreference: vi.fn(),
  },
}))

vi.mock("@/lib/watch-api", () => ({ watchApi: watchApiMock }))

const favorite: WatchRule = {
  id: "rule-favorite",
  userId: "user-1",
  deviceId: "pile-1",
  portId: null,
  notifyIdle: false,
  enabled: true,
  activeWeekdays: 127,
  activeStartMinute: 0,
  activeEndMinute: 0,
  timezone: "Asia/Shanghai",
  createdAt: "2026-08-10T00:00:00Z",
  updatedAt: "2026-08-10T00:00:00Z",
}

const overview: WatchOverview = {
  ruleCount: 1,
  ruleLimit: 20,
  reminderPileCount: 0,
  reminderPileLimit: 5,
  dailyQuotaUsed: 3,
  dailyQuotaLimit: 480,
  quotaDate: "2026-08-10",
  refreshIntervalMinutes: 10,
  backgroundRemindersEnabled: true,
  accountRefreshEnabled: true,
  scheduledPowerOffEnabled: true,
  scheduledPowerOffStartMinute: 1380,
  scheduledPowerOffEndMinute: 420,
  scheduledPowerOffTimezone: "Asia/Shanghai",
}

const preference: NotificationPreference = {
  userId: "user-1",
  browserEnabled: false,
  quietHoursEnabled: true,
  quietStartMinute: 1320,
  quietEndMinute: 480,
  timezone: "Asia/Shanghai",
  updatedAt: "2026-08-10T00:00:00Z",
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("WatchProvider", () => {
  it("deduplicates parallel loads and updates quota counts locally", async () => {
    watchApiMock.rules.mockResolvedValue([favorite])
    watchApiMock.overview.mockResolvedValue(overview)
    watchApiMock.preference.mockResolvedValue(preference)
    const reminder: WatchRule = {
      ...favorite,
      id: "rule-port",
      portId: 3,
      notifyIdle: true,
      updatedAt: "2026-08-10T00:01:00Z",
    }
    watchApiMock.createRule.mockResolvedValue(reminder)

    const { result } = renderHook(() => useWatch(), {
      wrapper: WatchProvider,
    })

    await act(async () => {
      await Promise.all([result.current.load(), result.current.load()])
    })

    expect(watchApiMock.rules).toHaveBeenCalledTimes(1)
    expect(watchApiMock.overview).toHaveBeenCalledTimes(1)
    expect(watchApiMock.preference).toHaveBeenCalledTimes(1)
    expect(result.current.loaded).toBe(true)

    await act(async () => {
      await result.current.createRule({
        deviceId: "pile-1",
        portId: 3,
        notifyIdle: true,
      })
    })

    expect(result.current.rules.map((rule) => rule.id)).toEqual([
      "rule-port",
      "rule-favorite",
    ])
    expect(result.current.overview).toMatchObject({
      ruleCount: 2,
      reminderPileCount: 1,
    })
    expect(watchApiMock.overview).toHaveBeenCalledTimes(1)
  })
})
