import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { WatchManagementSheet } from "@/components/watch-management-sheet"
import type { Pile } from "@/lib/types"

const { watchContextMock } = vi.hoisted(() => ({
  watchContextMock: {
    rules: [
      {
        id: "rule-1",
        userId: "user-1",
        deviceId: "pile-1",
        portId: 2,
        notifyIdle: true,
        enabled: true,
        activeWeekdays: 31,
        activeStartMinute: 480,
        activeEndMinute: 1320,
        timezone: "Asia/Shanghai",
        createdAt: "2026-08-10T00:00:00Z",
        updatedAt: "2026-08-10T00:00:00Z",
      },
    ],
    overview: {
      ruleCount: 1,
      ruleLimit: 20,
      reminderPileCount: 1,
      reminderPileLimit: 5,
      dailyQuotaUsed: 7,
      dailyQuotaLimit: 480,
      quotaDate: "2026-08-10",
      refreshIntervalMinutes: 10,
      backgroundRemindersEnabled: true,
      accountRefreshEnabled: true,
      scheduledPowerOffEnabled: true,
      scheduledPowerOffStartMinute: 1380,
      scheduledPowerOffEndMinute: 420,
      scheduledPowerOffTimezone: "Asia/Shanghai",
    },
    preference: {
      userId: "user-1",
      browserEnabled: false,
      quietHoursEnabled: true,
      quietStartMinute: 1320,
      quietEndMinute: 480,
      timezone: "Asia/Shanghai",
      updatedAt: "2026-08-10T00:00:00Z",
    },
    loading: false,
    loaded: true,
    load: vi.fn(),
    updateRule: vi.fn(),
    deleteRule: vi.fn(),
    updatePreference: vi.fn(),
  },
}))

vi.mock("@/lib/watch-context", () => ({
  useWatch: () => watchContextMock,
}))

const pile: Pile = {
  id: "pile-1",
  number: "61034278",
  name: "松园 3 号楼",
  status: "在线",
  address: "北门",
  openNum: 1,
  online: true,
  createdAt: "2026-08-10T00:00:00Z",
  updatedAt: "2026-08-10T00:00:00Z",
  source: "remote",
  usedPortIds: [2],
  ports: [
    {
      id: 2,
      status: "in_use",
      powerKw: 6.6,
      energyKwh: 2,
      updatedAt: "2026-08-10T00:00:00Z",
      sessionMin: 10,
      usedSeconds: 600,
    },
  ],
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe("WatchManagementSheet", () => {
  it("shows quotas, manages rule status, and saves quiet hours", async () => {
    const user = userEvent.setup()
    watchContextMock.updateRule.mockResolvedValue({})
    watchContextMock.deleteRule.mockResolvedValue(undefined)
    watchContextMock.updatePreference.mockResolvedValue({
      ...watchContextMock.preference,
      quietStartMinute: 1380,
      quietEndMinute: 420,
    })

    render(
      <WatchManagementSheet
        piles={[pile]}
        open
        onOpenChange={vi.fn()}
        onEditRule={vi.fn()}
      />
    )

    expect(screen.getByText("1/20")).toBeInTheDocument()
    expect(screen.getByText("7/480")).toBeInTheDocument()
    expect(screen.getByText("工作日 · 08:00–22:00")).toBeInTheDocument()

    await user.click(screen.getByRole("switch", { name: "停用规则" }))
    expect(watchContextMock.updateRule).toHaveBeenCalledWith("rule-1", {
      enabled: false,
    })

    await user.click(screen.getByRole("tab", { name: "提醒设置" }))
    expect(screen.getByText("23:00–07:00")).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText("开始时间"), {
      target: { value: "23:00" },
    })
    fireEvent.change(screen.getByLabelText("结束时间"), {
      target: { value: "07:00" },
    })
    await user.click(screen.getByRole("button", { name: "保存免打扰设置" }))

    await waitFor(() =>
      expect(watchContextMock.updatePreference).toHaveBeenCalledWith({
        quietHoursEnabled: true,
        quietStartMinute: 1380,
        quietEndMinute: 420,
        timezone: "Asia/Shanghai",
      })
    )

    await user.click(screen.getByRole("tab", { name: "关注列表" }))
    await user.click(screen.getByRole("button", { name: "删除" }))
    expect(screen.getByText("删除这条关注规则？")).toBeVisible()
    await user.click(screen.getByRole("button", { name: "确认删除" }))
    await waitFor(() =>
      expect(watchContextMock.deleteRule).toHaveBeenCalledWith("rule-1")
    )
  })
})
