import { readFile } from "node:fs/promises"
import { describe, expect, it } from "vitest"

describe("root providers", () => {
  it("does not import dashboard state for every route", async () => {
    const source = await readFile("components/providers.tsx", "utf8")

    expect(source).not.toContain("DashboardProvider")
  })
})
