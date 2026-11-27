import { describe, expect, it } from "vitest"

import { normalizeLegacyHash, resolveHomeRoute } from "@/lib/routing"

describe("resolveHomeRoute", () => {
  it("routes admins, users, and guests to their expected entry points", () => {
    expect(resolveHomeRoute("admin")).toBe("/admin")
    expect(resolveHomeRoute("user")).toBe("/dashboard")
    expect(resolveHomeRoute(null)).toBe("/login")
  })
})

describe("normalizeLegacyHash", () => {
  it("maps a historical hash route to its app-router path", () => {
    expect(normalizeLegacyHash("#/dashboard")).toBe("/dashboard")
  })
})
