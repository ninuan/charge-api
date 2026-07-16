import { describe, expect, it } from "vitest"

import nextConfig from "../next.config"

describe("static export configuration", () => {
  it("disables Next development indicators", () => {
    expect(nextConfig.devIndicators).toBe(false)
  })

  it("does not redirect API event streams to a trailing slash", () => {
    expect(nextConfig.skipTrailingSlashRedirect).toBe(true)
  })

  it("exports the application as static files", async () => {
    const config = (await import("../next.config")).default

    expect(config.output).toBe("export")
  })
})
