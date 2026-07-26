import { afterEach, describe, expect, it, vi } from "vitest"

import { request, requestJSON } from "@/lib/http"

describe("requestJSON", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it("sends credentialed requests without exposing a server error", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ error: "dial internal-yyb:8443 with token=secret" }),
          { status: 502 }
        )
      )
    vi.stubGlobal("fetch", fetchMock)

    await expect(
      requestJSON(
        "/api/piles",
        { method: "POST" },
        "添加充电桩失败，请检查桩号后重试。"
      )
    ).rejects.toThrow("添加充电桩失败，请检查桩号后重试。")

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/piles",
      expect.objectContaining({ credentials: "include", method: "POST" })
    )
  })

  it("shows an allowlisted business reason without trusting the server error text", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "PILE_NUMBER_INVALID",
              error: "dial internal-yyb:8443 with token=secret",
            }),
            { status: 400 }
          )
        )
    )

    await expect(
      requestJSON(
        "/api/piles",
        { method: "POST" },
        "添加充电桩失败，请检查桩号后重试。"
      )
    ).rejects.toThrow("桩号必须是 6-64 位数字")
  })

  it("shows an allowlisted login reason even when the API uses 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "AUTH_INVALID_CREDENTIALS",
              error: "database user record mismatch",
            }),
            { status: 401 }
          )
        )
    )

    await expect(
      requestJSON(
        "/api/auth/login",
        { method: "POST" },
        "登录失败，请稍后重试。"
      )
    ).rejects.toThrow("用户名或密码错误")
  })

  it("aborts requests that hang beyond the default timeout", async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(
        (_path: string, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener("abort", () =>
              reject(new DOMException("The operation was aborted.", "AbortError"))
            )
          })
      )
    )

    const pending = expect(
      request("/api/piles", {}, "暂时无法加载充电桩信息，请稍后重试。")
    ).rejects.toThrow("请求超时，请检查网络后重试")
    await vi.advanceTimersByTimeAsync(30_000)
    await pending
  })

  it("keeps slow remote operations alive within a widened timeout", async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(
        (_path: string, init: RequestInit) =>
          new Promise((resolve, reject) => {
            init.signal?.addEventListener("abort", () =>
              reject(new DOMException("The operation was aborted.", "AbortError"))
            )
            setTimeout(
              () =>
                resolve(
                  new Response(JSON.stringify({ ok: true }), { status: 200 })
                ),
              60_000
            )
          })
      )
    )

    const pending = request<{ ok: boolean }>(
      "/api/refresh",
      { method: "POST", timeoutMs: 120_000 },
      "暂时无法刷新设备状态，请稍后重试。"
    )
    await vi.advanceTimersByTimeAsync(60_000)
    await expect(pending).resolves.toEqual({ ok: true })
  })

  it("handles empty responses through the same request entry point", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    )

    await expect(
      request<void>("/api/piles/p1", { method: "DELETE" }, "删除失败")
    ).resolves.toBeUndefined()
  })
})
