import { describe, expect, it } from "vitest"

import { filterPiles } from "@/lib/dashboard-filters"
import type { Pile } from "@/lib/types"

const pile: Pile = {
  id: "61034278", number: "61034278", name: "松园北侧", status: "在线", address: "北门", openNum: 2, online: true,
  createdAt: "", updatedAt: "", source: "manual", ports: [
    { id: 1, status: "idle", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 },
    { id: 2, status: "in_use", powerKw: 0, energyKwh: 0, updatedAt: "", sessionMin: 0, usedSeconds: 0 },
  ], usedPortIds: [2],
}

describe("filterPiles", () => {
  it("retains only matching ports when searching by a port number", () => {
    expect(filterPiles([pile], "2", "all")).toEqual([{ pile, portIds: [2] }])
  })

  it("filters the existing in_use status without changing the data", () => {
    expect(filterPiles([pile], "", "charging")).toEqual([{ pile, portIds: [2] }])
  })
})
