import { describe, expect, it } from "vitest"

import { safeHref } from "./safe-href"

describe("safeHref", () => {
  it("allows valid https URLs", () => {
    expect(safeHref("https://www.bilibili.com/video/BV1xx411c7mD")).toBe(
      "https://www.bilibili.com/video/BV1xx411c7mD"
    )
    expect(safeHref("  https://example.com/guide?a=1#sec  ")).toBe(
      "https://example.com/guide?a=1#sec"
    )
  })

  it("blocks dangerous schemes and http", () => {
    expect(safeHref("javascript:alert(1)")).toBeNull()
    expect(safeHref("JAVASCRIPT:alert(1)")).toBeNull()
    expect(safeHref("data:text/html,<script>alert(1)</script>")).toBeNull()
    expect(safeHref("vbscript:msgbox(1)")).toBeNull()
    expect(safeHref("http://example.com/insecure")).toBeNull()
    expect(safeHref("https://example.com/has space")).toBeNull()
    expect(safeHref("")).toBeNull()
    expect(safeHref(undefined)).toBeNull()
  })

  it("handles relative paths only when allowRelative is true", () => {
    expect(safeHref("/account?tab=connection")).toBeNull()
    expect(safeHref("/account?tab=connection", { allowRelative: true })).toBe(
      "/account?tab=connection"
    )
    expect(safeHref("//evil.example/phish", { allowRelative: true })).toBeNull()
    expect(safeHref("/\\evil.example", { allowRelative: true })).toBeNull()
  })
})
