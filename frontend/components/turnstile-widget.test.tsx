import { cleanup, render, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { TurnstileWidget } from "@/components/turnstile-widget"

describe("TurnstileWidget", () => {
  afterEach(() => {
    cleanup()
    delete window.turnstile
  })

  it("does not recreate the provider widget when callback props change", async () => {
    const providerRender = vi.fn(() => "widget-1")
    window.turnstile = { render: providerRender, reset: vi.fn(), remove: vi.fn() }

    const { rerender } = render(
      <TurnstileWidget action="login" siteKey="site" onVerified={vi.fn()} onExpired={vi.fn()} onError={vi.fn()} />
    )

    await waitFor(() => expect(providerRender).toHaveBeenCalledTimes(1))

    rerender(
      <TurnstileWidget action="login" siteKey="site" onVerified={vi.fn()} onExpired={vi.fn()} onError={vi.fn()} />
    )

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(providerRender).toHaveBeenCalledTimes(1)
  })
})
