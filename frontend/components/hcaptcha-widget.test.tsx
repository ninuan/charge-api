import { cleanup, render, waitFor } from "@testing-library/react"
import { createRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

const themeState = vi.hoisted(() => ({ value: "light" }))
vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: themeState.value }),
}))

import {
  HCaptchaWidget,
  type HCaptchaWidgetHandle,
} from "@/components/hcaptcha-widget"

describe("HCaptchaWidget", () => {
  afterEach(() => {
    cleanup()
    themeState.value = "light"
    delete window.hcaptcha
    delete window.__chargeHCaptchaReady
    document
      .querySelectorAll("script[data-hcaptcha-script]")
      .forEach((script) => script.remove())
  })

  it("does not recreate the provider widget when callback props change", async () => {
    const providerRender = vi.fn(() => "widget-1")
    window.hcaptcha = {
      render: providerRender,
      reset: vi.fn(),
      remove: vi.fn(),
    }

    const { rerender } = render(
      <HCaptchaWidget
        siteKey="site"
        onVerified={vi.fn()}
        onExpired={vi.fn()}
        onError={vi.fn()}
      />
    )

    await waitFor(() => expect(providerRender).toHaveBeenCalledTimes(1))

    rerender(
      <HCaptchaWidget
        siteKey="site"
        onVerified={vi.fn()}
        onExpired={vi.fn()}
        onError={vi.fn()}
      />
    )

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(providerRender).toHaveBeenCalledTimes(1)
  })

  it("recreates the provider widget with the active color theme", async () => {
    const providerRender = vi
      .fn()
      .mockReturnValueOnce("widget-light")
      .mockReturnValueOnce("widget-dark")
    const remove = vi.fn()
    window.hcaptcha = {
      render: providerRender,
      reset: vi.fn(),
      remove,
    }
    const props = {
      siteKey: "site",
      onVerified: vi.fn(),
      onExpired: vi.fn(),
      onError: vi.fn(),
    }
    const { rerender } = render(<HCaptchaWidget {...props} />)
    await waitFor(() => expect(providerRender).toHaveBeenCalledTimes(1))
    expect(providerRender.mock.calls[0][1]).toMatchObject({ theme: "light" })

    themeState.value = "dark"
    rerender(<HCaptchaWidget {...props} />)

    await waitFor(() => expect(providerRender).toHaveBeenCalledTimes(2))
    expect(remove).toHaveBeenCalledWith("widget-light")
    expect(providerRender.mock.calls[1][1]).toMatchObject({ theme: "dark" })
  })

  it("exposes reset and forwards verification callbacks", async () => {
    const reset = vi.fn()
    const onVerified = vi.fn()
    const onExpired = vi.fn()
    const onError = vi.fn()
    let options: Record<string, unknown> = {}
    window.hcaptcha = {
      render: vi.fn((_element, providerOptions) => {
        options = providerOptions
        return "widget-2"
      }),
      reset,
      remove: vi.fn(),
    }
    const ref = createRef<HCaptchaWidgetHandle>()

    render(
      <HCaptchaWidget
        ref={ref}
        siteKey="site"
        onVerified={onVerified}
        onExpired={onExpired}
        onError={onError}
      />
    )
    await waitFor(() => expect(window.hcaptcha?.render).toHaveBeenCalled())

    ;(options.callback as (token: string) => void)("verified-token")
    ;(options["expired-callback"] as () => void)()
    ;(options["error-callback"] as () => void)()
    ref.current?.reset()

    expect(onVerified).toHaveBeenCalledWith("verified-token")
    expect(onExpired).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledTimes(1)
    expect(reset).toHaveBeenCalledWith("widget-2")
  })

  it("reports a provider script loading failure", async () => {
    const onError = vi.fn()
    render(
      <HCaptchaWidget
        siteKey="site"
        onVerified={vi.fn()}
        onExpired={vi.fn()}
        onError={onError}
      />
    )

    const script = document.querySelector<HTMLScriptElement>(
      "script[data-hcaptcha-script]"
    )
    expect(script?.src).toContain("https://js.hcaptcha.com/1/api.js")
    script?.dispatchEvent(new Event("error"))

    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
  })

  it("waits for the official SDK callback before rendering", async () => {
    const providerRender = vi.fn(() => "widget-ready")
    render(
      <HCaptchaWidget
        siteKey="site"
        onVerified={vi.fn()}
        onExpired={vi.fn()}
        onError={vi.fn()}
      />
    )

    await waitFor(() =>
      expect(window.__chargeHCaptchaReady).toBeTypeOf("function")
    )
    window.hcaptcha = {
      render: providerRender,
      reset: vi.fn(),
      remove: vi.fn(),
    }
    expect(providerRender).not.toHaveBeenCalled()
    window.__chargeHCaptchaReady?.()

    await waitFor(() => expect(providerRender).toHaveBeenCalledTimes(1))
  })
})
