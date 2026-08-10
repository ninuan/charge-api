import { afterEach, describe, expect, it, vi } from "vitest"

import { watchApi } from "@/lib/watch-api"

describe("watchApi", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("loads all watch management resources from their scoped endpoints", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(async () => new Response(JSON.stringify([])))
    vi.stubGlobal("fetch", fetchMock)

    await Promise.all([
      watchApi.rules(),
      watchApi.overview(),
      watchApi.preference(),
    ])

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/watch-rules",
      expect.objectContaining({ credentials: "include" })
    )
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/watch-overview",
      expect.objectContaining({ credentials: "include" })
    )
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notification-preferences",
      expect.objectContaining({ credentials: "include" })
    )
  })

  it("writes one whole-pile reminder without a port target", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({})))
    vi.stubGlobal("fetch", fetchMock)

    await watchApi.createRule({
      deviceId: "pile-1",
      activeWeekdays: 127,
      activeStartMinute: 0,
      activeEndMinute: 0,
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/watch-rules",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          deviceId: "pile-1",
          activeWeekdays: 127,
          activeStartMinute: 0,
          activeEndMinute: 0,
        }),
      })
    )
  })
})
