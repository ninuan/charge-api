import { afterEach, describe, expect, it, vi } from "vitest"

import { requestJSON } from "@/lib/http"

describe("requestJSON", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("sends credentialed requests without exposing a server error", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: "dial internal-yyb:8443 with token=secret" }), { status: 502 })
    )
    vi.stubGlobal("fetch", fetchMock)

    await expect(
      requestJSON("/api/piles", { method: "POST" }, "添加充电桩失败，请检查桩号后重试。")
    ).rejects.toThrow("添加充电桩失败，请检查桩号后重试。")

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/piles",
      expect.objectContaining({ credentials: "include", method: "POST" })
    )
  })

  it("shows an allowlisted business reason without trusting the server error text", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "PILE_NUMBER_INVALID", error: "dial internal-yyb:8443 with token=secret" }), { status: 400 })
    ))

    await expect(
      requestJSON("/api/piles", { method: "POST" }, "添加充电桩失败，请检查桩号后重试。")
    ).rejects.toThrow("桩号必须是 6-64 位数字")
  })

  it("shows an allowlisted login reason even when the API uses 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "AUTH_INVALID_CREDENTIALS", error: "database user record mismatch" }), { status: 401 })
    ))

    await expect(requestJSON("/api/auth/login", { method: "POST" }, "登录失败，请稍后重试。")).rejects.toThrow("用户名或密码错误")
  })
})
