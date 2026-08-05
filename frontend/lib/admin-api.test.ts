import { afterEach, describe, expect, it, vi } from "vitest"

import { adminApi } from "@/lib/admin-api"

describe("adminApi invites", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("requests one bounded invite page", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [],
          page: 2,
          pageSize: 20,
          total: 41,
          totalPages: 3,
        })
      )
    )
    vi.stubGlobal("fetch", fetchMock)

    await adminApi.invites({ page: 2, pageSize: 20 })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/invites?page=2&pageSize=20",
      expect.objectContaining({ credentials: "include" })
    )
  })
})

describe("adminApi trends", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("keeps range and timezone aligned for JSON and CSV requests", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ points: [] })))
      .mockResolvedValueOnce(
        new Response("\uFEFF时间段开始,请求量\n", {
          headers: {
            "Content-Type": "text/csv; charset=utf-8",
            "Content-Disposition":
              'attachment; filename="charge-trends-7d-2026-08-05.csv"',
          },
        })
      )
    vi.stubGlobal("fetch", fetchMock)

    await adminApi.trends("7d", "UTC")
    const file = await adminApi.trendsCSV("7d", "UTC")

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/admin/trends?range=7d&timezone=UTC",
      expect.objectContaining({ credentials: "include" })
    )
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/admin/trends.csv?range=7d&timezone=UTC",
      expect.objectContaining({ credentials: "include" })
    )
    expect(file.filename).toBe("charge-trends-7d-2026-08-05.csv")
    expect(file.blob.type).toBe("text/csv;charset=utf-8")
  })

  it("uses the stable public error for a failed CSV export", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "ADMIN_TRENDS_UNAVAILABLE" }), {
          status: 503,
        })
      )
    )

    await expect(adminApi.trendsCSV("30d")).rejects.toThrow(
      "运营趋势暂时不可用，请稍后重试"
    )
  })
})
