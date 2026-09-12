import { describe, expect, it } from "vitest"
import {
  dashboardHref,
  formatSnapshotTime,
  portCounts,
  portTime,
  selectWorkbenchPiles,
  updatePageQuery,
} from "./workbench"
import type { Pile } from "./types"

const pile: Pile = {
  id: "12345678901",
  number: "608001",
  name: "北门车棚",
  address: "入口右侧",
  status: "在线",
  online: true,
  openNum: 2,
  source: "manual",
  createdAt: "",
  updatedAt: "",
  usedPortIds: [2],
  ports: [
    {
      id: 1,
      status: "idle",
      powerKw: 0,
      energyKwh: 0,
      updatedAt: "",
      sessionMin: 0,
      usedSeconds: 0,
    },
    {
      id: 2,
      status: "in_use",
      powerKw: 0.22,
      energyKwh: 0.4,
      updatedAt: "",
      sessionMin: 120,
      usedSeconds: 3600,
    },
  ],
}
describe("live workbench domain", () => {
  it("preserves unknown devices in the all list instead of treating them as idle", () => {
    const unknown = { ...pile, ports: [], openNum: 0, online: false }
    expect(selectWorkbenchPiles([unknown], "", "all")).toEqual([
      { pile: unknown, portIds: [] },
    ])
    expect(selectWorkbenchPiles([unknown], "", "idle")).toEqual([])
  })
  it("finds ports, long IDs, names and positions without altering readings", () => {
    expect(selectWorkbenchPiles([pile], "02", "all")[0].portIds).toEqual([2])
    expect(selectWorkbenchPiles([pile], "入口", "all")[0].pile).toBe(pile)
    expect(selectWorkbenchPiles([pile], pile.id, "all")).toHaveLength(1)
    expect(portCounts(pile)).toEqual({ idle: 1, in_use: 1, offline: 0 })
  })
  it("never promotes a disconnected pile's cached idle port as available", () => {
    expect(
      selectWorkbenchPiles([{ ...pile, online: false }], "", "idle")
    ).toEqual([])
  })
  it("does not invent remaining time when the platform omits it", () => {
    expect(portTime(pile.ports[1], "remaining")).toBe("平台未提供")
    expect(portTime(pile.ports[1], "used")).toBe("1 小时 0 分钟")
    expect(portTime(pile.ports[0], "used")).toBe("—")
  })
  it("encodes object links and preserves independent query fields", () => {
    expect(dashboardHref(pile.id, 2)).toBe(
      "/dashboard?pile=12345678901&port=2"
    )
    window.history.replaceState(null, "", "/dashboard?q=北门&status=idle")
    updatePageQuery({ pile: pile.id, port: 2 })
    expect(new URLSearchParams(window.location.search).get("q")).toBe("北门")
    updatePageQuery({ pile: null, port: null })
    expect(new URLSearchParams(window.location.search).has("pile")).toBe(false)
  })
  it("renders a missing or malformed timestamp as unknown", () => {
    expect(formatSnapshotTime()).toBe("尚未读取")
    expect(formatSnapshotTime("invalid")).toBe("尚未读取")
  })
})
