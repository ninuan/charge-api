import { describe, expect, it } from "vitest"

import { normalizeLegacyHash, resolveRoute } from "@/lib/routing"

describe("resolveRoute", () => {
  it("sends a user away from an administrator-only page", () => {
    expect(resolveRoute("user", "admin")).toBe("/dashboard")
  })

  it("sends an anonymous visitor to the login page", () => {
    expect(resolveRoute(null, "user")).toBe("/login")
  })
})

describe("normalizeLegacyHash", () => {
  it("maps a historical hash route to its app-router path", () => {
    expect(normalizeLegacyHash("#/dashboard")).toBe("/dashboard")
  })
})
