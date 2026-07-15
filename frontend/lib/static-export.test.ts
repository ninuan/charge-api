import { describe, expect, it } from "vitest"

describe("static export configuration", () => {
  it("exports the application as static files", async () => {
    const config = (await import("../next.config")).default

    expect(config.output).toBe("export")
  })
})
