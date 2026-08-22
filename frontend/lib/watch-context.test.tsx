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

const pileRule: WatchRule = {
  id: "rule-pile-1",
  userId: "user-1",
  deviceId: "pile-1",
  mode: "recurring",
  enabled: true,
  activeWeekdays: 127,
  activeStartMinute: 0,
  activeEndMinute: 0,
  timezone: "Asia/Shanghai",
  stopAfterNotify: false,
  createdAt: "2026-08-10T00:00:00Z",
  updatedAt: "2026-08-10T00:00:00Z",
}

const overview: WatchOverview = {
  reminderPileCount: 1,
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
    watchApiMock.rules.mockResolvedValue([pileRule])
    watchApiMock.overview.mockResolvedValue(overview)
    watchApiMock.preference.mockResolvedValue(preference)
    const reminder: WatchRule = {
      ...pileRule,
      id: "rule-pile-2",
      deviceId: "pile-2",
      updatedAt: "2026-08-10T00:01:00Z",
    }
    watchApiMock.createRule.mockResolvedValue({
      rule: reminder,
      idlePortIds: [],
      backgroundScheduled: true,
      message: "固定时段提醒已保存。",
    })

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
        deviceId: "pile-2",
      })
    })

    expect(result.current.rules.map((rule) => rule.id)).toEqual([
      "rule-pile-2",
      "rule-pile-1",
    ])
    expect(result.current.overview).toMatchObject({
      reminderPileCount: 2,
    })
    expect(watchApiMock.overview).toHaveBeenCalledTimes(1)
  })
})
