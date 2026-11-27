import { describe, expect, it } from "vitest"

import { parseDashboardQuery } from "@/lib/dashboard-query"

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
})
