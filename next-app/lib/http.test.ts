import { afterEach, describe, expect, it, vi } from "vitest"

import { requestJSON } from "@/lib/http"

describe("requestJSON", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("sends credentialed requests and surfaces the server error", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: "拒绝访问" }), { status: 403 })
    )
    vi.stubGlobal("fetch", fetchMock)

    await expect(
      requestJSON("/api/piles", { method: "POST" }, "保存失败")
    ).rejects.toThrow("拒绝访问")

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/piles",
      expect.objectContaining({ credentials: "include", method: "POST" })
    )
  })
})
