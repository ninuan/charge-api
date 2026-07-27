import { describe, expect, it } from "vitest"

import {
  parseDashboardQuery,
  serializeDashboardQuery,
} from "@/lib/dashboard-query"

describe("dashboard query", () => {
  it("restores search and a supported port status", () => {
    expect(parseDashboardQuery("?q=%E6%9D%BE%E5%9B%AD&status=idle")).toEqual({
      search: "松园",
      filter: "idle",
    })
  })

  it("falls back to all for an unsupported status", () => {
    expect(parseDashboardQuery("?status=broken")).toEqual({
      search: "",
      filter: "all",
    })
  })

  it("updates dashboard fields while preserving unrelated query fields", () => {
    expect(
      serializeDashboardQuery(
        { search: " 03 ", filter: "charging" },
        "?source=shortcut&status=idle"
      )
    ).toBe("source=shortcut&status=charging&q=03")
  })

  it("removes default dashboard fields from the URL", () => {
    expect(
      serializeDashboardQuery(
        { search: " ", filter: "all" },
        "?q=old&status=offline"
      )
    ).toBe("")
  })
})
