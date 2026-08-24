import { afterEach, describe, expect, it, vi } from "vitest"

import { testWxPusherChannel, updateWxPusherChannel } from "@/lib/wxpusher-api"

describe("wxpusher api", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("updates channel preferences with PATCH", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ bound: true })))
    vi.stubGlobal("fetch", fetchMock)

    await updateWxPusherChannel({
      enabled: true,
      eventTypes: ["pile_available", "pile_recovered"],
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notification-channels/wxpusher",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          enabled: true,
          eventTypes: ["pile_available", "pile_recovered"],
        }),
      })
    )
  })

  it("queues a test delivery", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: "ndl_test" })))
    vi.stubGlobal("fetch", fetchMock)

    await testWxPusherChannel()

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notification-channels/wxpusher/test",
      expect.objectContaining({ method: "POST" })
    )
  })
})
