import { describe, expect, it } from "vitest"
import {
  counts,
  filterPiles,
  historyBuckets,
  initialData,
  isActionNotice,
  parseRoute,
  routeHref,
  safeData,
  SNAPSHOT_AT,
  validateIdentifier,
  watchEnd,
} from "./model"

describe("prototype domain boundaries", () => {
  it("keeps ports and aggregates consistent", () => {
    for (const pile of initialData().piles) {
      const total = counts(pile)
      expect(total.idle + total.in_use + total.offline).toBe(pile.openNum)
      expect(pile.usedPortIds).toEqual(
        pile.ports.filter((p) => p.status === "in_use").map((p) => p.id)
      )
      expect(pile.source).toBe("prototype-fixture")
    }
    expect(counts(initialData().piles[0])).toEqual({
      idle: 4,
      in_use: 5,
      offline: 1,
    })
  })
  it("searches names, numbers, positions and long device IDs", () => {
    const { piles } = initialData()
    for (const query of ["北门", "608201", "第一排", "202609100001"])
      expect(filterPiles(piles, query, "all").map((p) => p.id)).toEqual([
        piles[0].id,
      ])
  })
  it("does not count disconnected devices as available", () => {
    const { piles } = initialData()
    piles[0].online = false
    expect(filterPiles(piles, "", "idle")).toHaveLength(1)
    expect(filterPiles(piles, "", "offline")).toHaveLength(2)
    expect(filterPiles(piles, "", "busy")).toHaveLength(1)
  })
  it.each(["12345", "abcdefgh", "123 456", "1".repeat(65)])(
    "rejects invalid identifier %s",
    (id) => {
      expect(validateIdentifier(id, [])).not.toBe("")
    }
  )
  it("rejects duplicate pile number and long ID", () => {
    const { piles } = initialData()
    expect(validateIdentifier(piles[0].number, piles)).toContain("已经")
    expect(validateIdentifier(piles[0].id, piles)).toContain("已经")
    expect(validateIdentifier(" 608205 ", piles)).toBe("")
  })
  it("keeps informational notices out of the issues lifecycle", () => {
    const { notices } = initialData()
    expect(notices.filter(isActionNotice).map((n) => n.type)).toEqual([
      "pile_offline",
      "credential_expired",
    ])
    notices[1].read = true
    expect(notices[1].resolved).toBe(false)
  })
  it("caps temporary reminders at power-off, in Shanghai time", () => {
    expect(watchEnd("4h", "2026-09-10T22:00:00+08:00", true)).toBe(
      "2026-09-10T15:00:00.000Z"
    )
    expect(watchEnd("4h", "2026-09-10T22:00:00+08:00", false)).toBe(
      "2026-09-10T18:00:00.000Z"
    )
    expect(watchEnd("until_power_off", SNAPSHOT_AT, true)).toBe(
      "2026-09-10T15:00:00.000Z"
    )
  })
  it("round-trips object, tab and port deep links", () => {
    const route = {
      view: "piles" as const,
      id: "202609100001",
      tab: "history",
      port: 3,
    }
    expect(parseRoute(routeHref(route))).toEqual(route)
    expect(parseRoute("#/unknown").view).toBe("piles")
    expect(parseRoute("#/piles/123456?port=-1").port).toBeUndefined()
  })
  it.each([
    ["24h", 24],
    ["7d", 7],
    ["30d", 30],
  ])("uses only past observations in %s", (range, count) => {
    const points = historyBuckets(String(range))
    expect(points).toHaveLength(Number(count))
    for (const point of points) {
      expect(new Date(point.at).getTime()).toBeLessThanOrEqual(
        new Date(SNAPSHOT_AT).getTime()
      )
      expect(
        point.idle * point.observed + point.busy + point.unknown
      ).toBeCloseTo(1)
      expect(point.unknown).toBeGreaterThan(0)
    }
  })
  it("falls back safely when saved prototype data is invalid", () => {
    expect(safeData("not json")).toEqual(initialData())
    expect(safeData("null")).toEqual(initialData())
    expect(safeData(JSON.stringify({ ...initialData(), piles: [{}] }))).toEqual(
      initialData()
    )
  })
  it("adds optional local preferences when reading older v2 data", () => {
    const old = { ...initialData(), reduceMotion: undefined }
    expect(safeData(JSON.stringify(old)).reduceMotion).toBe(false)
  })
})
