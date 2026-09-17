import { afterEach, expect, it, vi } from "vitest"
import { announcementRequest } from "./announcements"
afterEach(() => vi.unstubAllGlobals())
it.each(["POST", "PATCH", "DELETE"])(
  "%s announcement mutations declare JSON",
  async (method) => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response('{"ok":true}', { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await announcementRequest("/api/admin/announcements", {
      method,
      body: '{"version":1}',
      headers: { "X-Test": "preserved" },
    })
    const headers = new Headers(fetchMock.mock.calls[0][1].headers)
    expect(headers.get("Content-Type")).toBe("application/json")
    expect(headers.get("X-Test")).toBe("preserved")
  }
)
