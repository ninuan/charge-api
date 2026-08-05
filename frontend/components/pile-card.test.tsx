import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { PileCard } from "@/components/pile-card"
import type { Pile } from "@/lib/types"

const pile: Pile = {
  id: "pile-1",
  number: "61034278",
  name: "松园 3 号楼",
  status: "在线",
  address: "北门",
  openNum: 2,
  online: true,
  createdAt: "2026-07-15T00:00:00Z",
  updatedAt: "2026-07-15T00:00:00Z",
  source: "remote",
  ports: [
    {
      id: 1,
      status: "idle",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "2026-07-15T00:00:00Z",
      sessionMin: 0,
      usedSeconds: 0,
    },
    {
      id: 2,
      status: "offline",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "2026-07-15T00:00:00Z",
      sessionMin: 0,
      usedSeconds: 0,
    },
  ],
  usedPortIds: [],
  sortOrder: 0,
}

afterEach(cleanup)

describe("PileCard", () => {
  it("shows distinct idle and offline guidance", () => {
    render(
      <PileCard
        pile={pile}
        visiblePortIds={[1, 2]}
        filtering={false}
        canMoveUp={false}
        canMoveDown
        reordering={false}
        onMove={vi.fn()}
        onHistory={vi.fn()}
        onRemove={vi.fn()}
        onUpdate={vi.fn()}
      />
    )

    expect(screen.getByText("等待使用")).toBeInTheDocument()
    expect(screen.getByText("设备暂不可访问")).toBeInTheDocument()
  })

  it("exposes direct move controls and disables unavailable directions", async () => {
    const onMove = vi.fn()
    render(
      <PileCard
        pile={pile}
        visiblePortIds={[1, 2]}
        filtering={false}
        canMoveUp={false}
        canMoveDown
        reordering={false}
        onMove={onMove}
        onHistory={vi.fn()}
        onRemove={vi.fn()}
        onUpdate={vi.fn()}
      />
    )

    expect(screen.getByRole("button", { name: "上移充电桩" })).toBeDisabled()
    await userEvent.click(screen.getByRole("button", { name: "下移充电桩" }))
    expect(onMove).toHaveBeenCalledWith("pile-1", "down")
  })

  it("only highlights a port after its status actually changes", () => {
    const props = {
      visiblePortIds: [1, 2],
      filtering: false,
      canMoveUp: false,
      canMoveDown: true,
      reordering: false,
      onMove: vi.fn(),
      onHistory: vi.fn(),
      onRemove: vi.fn(),
      onUpdate: vi.fn(),
    }
    const { rerender } = render(<PileCard pile={pile} {...props} />)
    const firstPort = screen.getByLabelText("1 号充电口")
    expect(firstPort).toHaveClass("port-card-enter")
    expect(firstPort).not.toHaveClass("port-state-changed")

    rerender(
      <PileCard
        {...props}
        pile={{
          ...pile,
          ports: pile.ports.map((port) =>
            port.id === 1 ? { ...port, status: "in_use" as const } : port
          ),
        }}
      />
    )

    expect(firstPort).toHaveClass("port-state-changed")
    expect(screen.getByLabelText("2 号充电口")).not.toHaveClass(
      "port-state-changed"
    )
  })

  it("opens history from an explicit card action", async () => {
    const onHistory = vi.fn()
    render(
      <PileCard
        pile={pile}
        visiblePortIds={[1, 2]}
        filtering={false}
        canMoveUp={false}
        canMoveDown
        reordering={false}
        onMove={vi.fn()}
        onHistory={onHistory}
        onRemove={vi.fn()}
        onUpdate={vi.fn()}
      />
    )

    await userEvent.click(screen.getByRole("button", { name: "历史趋势" }))
    expect(onHistory).toHaveBeenCalledWith("pile-1")
  })
})
