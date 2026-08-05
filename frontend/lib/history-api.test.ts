import { afterEach, describe, expect, it, vi } from "vitest"

import { historyApi } from "@/lib/history-api"

describe("historyApi", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("requests device history with generated query parameters", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ historyNotice: "ready" }))
      )
    vi.stubGlobal("fetch", fetchMock)

    await historyApi.device("2601201412385560088", {
      range: "7d",
      timezone: "Asia/Shanghai",
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/piles/2601201412385560088/history?range=7d&timezone=Asia%2FShanghai",
      expect.objectContaining({ credentials: "include" })
    )
  })

  it("requests one port without appending an empty query string", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ historyNotice: "ready" }))
      )
    vi.stubGlobal("fetch", fetchMock)

    await historyApi.port("2601201412385560088", 3)

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/piles/2601201412385560088/ports/3/history",
      expect.objectContaining({ credentials: "include" })
    )
  })
})
