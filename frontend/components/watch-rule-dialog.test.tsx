import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { WatchRuleDialog } from "@/components/watch-rule-dialog"
import type { Pile } from "@/lib/types"

const { watchContextMock } = vi.hoisted(() => ({
  watchContextMock: {
    rules: [],
    overview: {
      reminderPileCount: 0,
      reminderPileLimit: 5,
      dailyQuotaUsed: 0,
      dailyQuotaLimit: 480,
      quotaDate: "2026-08-10",
      refreshIntervalMinutes: 10,
      backgroundRemindersEnabled: true,
      accountRefreshEnabled: true,
      scheduledPowerOffEnabled: false,
      scheduledPowerOffStartMinute: 1380,
      scheduledPowerOffEndMinute: 420,
      scheduledPowerOffTimezone: "Asia/Shanghai",
    },
    createRule: vi.fn(),
    updateRule: vi.fn(),
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
  openNum: 2,
  online: true,
  createdAt: "2026-08-10T00:00:00Z",
  updatedAt: "2026-08-10T00:00:00Z",
  source: "remote",
  usedPortIds: [2],
  ports: [
    {
      id: 1,
      status: "idle",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "2026-08-10T00:00:00Z",
      sessionMin: 0,
      usedSeconds: 0,
    },
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

describe("WatchRuleDialog", () => {
  it("creates a two-hour temporary reminder by default", async () => {
    const user = userEvent.setup()
    watchContextMock.createRule.mockResolvedValue({
      rule: { id: "rule-1" },
      idlePortIds: [],
      backgroundScheduled: true,
      message: "临时提醒已开始。",
    })
    const onOpenChange = vi.fn()

    render(
      <WatchRuleDialog
        piles={[pile]}
        target={{ pileId: "pile-1" }}
        open
        onOpenChange={onOpenChange}
      />
    )

    expect(screen.getByRole("tab", { name: "临时提醒" })).toHaveAttribute(
      "data-active"
    )
    expect(screen.getByLabelText("等待多久")).toHaveTextContent("2 小时")
    expect(screen.getByText(/后台最多约检查 12 次/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "开始提醒" }))

    await waitFor(() =>
      expect(watchContextMock.createRule).toHaveBeenCalledWith({
        deviceId: "pile-1",
        duration: "2h",
      })
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("shows idle ports immediately without leaving a background task", async () => {
    const user = userEvent.setup()
    watchContextMock.createRule.mockResolvedValue({
      idlePortIds: [3, 7],
      backgroundScheduled: false,
      message: "当前已有空闲口。",
    })
    const onOpenChange = vi.fn()

    render(
      <WatchRuleDialog
        piles={[pile]}
        target={{ pileId: "pile-1" }}
        open
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole("button", { name: "开始提醒" }))
    expect(await screen.findByText("3 号口、7 号口")).toBeVisible()
    expect(
      screen.getByText("已完成本次查询，不会创建后台提醒任务。")
    ).toBeVisible()
    expect(onOpenChange).not.toHaveBeenCalled()
  })

  it("keeps cross-midnight recurring reminders in the advanced tab", async () => {
    const user = userEvent.setup()
    watchContextMock.createRule.mockResolvedValue({
      rule: { id: "rule-1" },
      idlePortIds: [],
      backgroundScheduled: true,
      message: "固定时段提醒已保存。",
    })
    const onOpenChange = vi.fn()

    render(
      <WatchRuleDialog
        piles={[pile]}
        target={{ pileId: "pile-1" }}
        open
        onOpenChange={onOpenChange}
      />
    )

    await user.click(screen.getByRole("tab", { name: "固定时段（高级）" }))
    fireEvent.change(screen.getByLabelText("开始时间"), {
      target: { value: "22:30" },
    })
    fireEvent.change(screen.getByLabelText("结束时间"), {
      target: { value: "06:30" },
    })
    expect(screen.getByText(/22:30–06:30（跨午夜）/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "创建固定提醒" }))

    await waitFor(() =>
      expect(watchContextMock.createRule).toHaveBeenCalledWith({
        deviceId: "pile-1",
        mode: "recurring",
        enabled: true,
        activeWeekdays: 127,
        activeStartMinute: 1350,
        activeEndMinute: 390,
        timezone: "Asia/Shanghai",
      })
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
