import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AuthProvider, useAuth } from "@/lib/auth-context"

const user = {
  id: "u1",
  username: "alice",
  role: "user" as const,
  enabled: true,
  createdAt: "2026-07-15T00:00:00Z",
  deviceLimit: 3,
  refreshEnabled: true,
}

describe("AuthProvider", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("posts the unchanged login body and stores the returned user", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(user), { status: 200 })
    )
    vi.stubGlobal("fetch", fetchMock)
    const { result } = renderHook(() => useAuth(), { wrapper: AuthProvider })

    await act(() => result.current.login("alice", "secret", "captcha-token"))

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/login",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({ username: "alice", password: "secret", captchaToken: "captcha-token" }),
      })
    )
    expect(result.current.currentUser).toEqual(user)
  })
})
