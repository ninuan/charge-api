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
      duration: "2h",
    })

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/watch-rules",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          deviceId: "pile-1",
          duration: "2h",
        }),
      })
    )
  })

  it("pages and manages durable notifications", async () => {
    const fetchMock = vi.fn().mockImplementation(async (path: string) => {
      if (path.startsWith("/api/notifications?"))
        return new Response(
          JSON.stringify({ items: [], unreadCount: 0, nextCursor: "next-1" })
        )
      if (path.endsWith("/read-all"))
        return new Response(JSON.stringify({ updated: 2 }))
      if (path.endsWith("/resolved"))
        return new Response(JSON.stringify({ deleted: 1 }))
      return new Response(JSON.stringify({ id: "notice-1", readAt: "now" }))
    })
    vi.stubGlobal("fetch", fetchMock)

    await watchApi.notifications({
      status: "unread",
      cursor: "cursor-1",
      limit: 10,
    })
    await watchApi.markNotificationRead("notice/1")
    await watchApi.markAllNotificationsRead()
    await watchApi.clearResolvedNotifications()

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notifications?status=unread&limit=10&cursor=cursor-1",
      expect.objectContaining({ credentials: "include" })
    )
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notifications/notice%2F1/read",
      expect.objectContaining({ method: "POST" })
    )
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notifications/read-all",
      expect.objectContaining({ method: "POST" })
    )
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/notifications/resolved",
      expect.objectContaining({ method: "DELETE" })
    )
  })
})
