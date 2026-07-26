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
