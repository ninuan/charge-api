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
  it("creates a port reminder with an explicit cross-midnight window", async () => {
    const user = userEvent.setup()
    watchContextMock.createRule.mockResolvedValue({ id: "rule-1" })
    const onOpenChange = vi.fn()

    render(
      <WatchRuleDialog
        piles={[pile]}
        target={{ pileId: "pile-1", portId: 2 }}
        open
        onOpenChange={onOpenChange}
      />
    )

    fireEvent.change(screen.getByLabelText("开始时间"), {
      target: { value: "22:30" },
    })
    fireEvent.change(screen.getByLabelText("结束时间"), {
      target: { value: "06:30" },
    })
    expect(screen.getByText(/22:30–06:30（跨午夜）/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "创建规则" }))

    await waitFor(() =>
      expect(watchContextMock.createRule).toHaveBeenCalledWith({
        deviceId: "pile-1",
        portId: 2,
        notifyIdle: true,
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
