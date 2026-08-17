"use client"

import { forwardRef, useEffect, useImperativeHandle, useRef } from "react"
import { useTheme } from "next-themes"

type HCaptchaApi = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string
  reset: (widgetId?: string) => void
  remove: (widgetId: string) => void
}

declare global {
  interface Window {
    hcaptcha?: HCaptchaApi
    __chargeHCaptchaReady?: () => void
  }
}

export type HCaptchaWidgetHandle = { reset: () => void }

type HCaptchaWidgetProps = {
  siteKey: string
  onVerified: (token: string) => void
  onExpired: () => void
  onError: () => void
}

let hcaptchaScriptPromise: Promise<void> | null = null

function loadHCaptchaScript() {
  if (window.hcaptcha) return Promise.resolve()
  if (!document.querySelector("script[data-hcaptcha-script]"))
    hcaptchaScriptPromise = null
  if (hcaptchaScriptPromise) return hcaptchaScriptPromise

  hcaptchaScriptPromise = new Promise<void>((resolve, reject) => {
    window.__chargeHCaptchaReady = () => {
      delete window.__chargeHCaptchaReady
      resolve()
    }
    const existing = document.querySelector<HTMLScriptElement>(
      "script[data-hcaptcha-script]"
    )
    if (existing) {
      existing.addEventListener(
        "error",
        () => reject(new Error("hCaptcha script failed")),
        { once: true }
      )
      return
    }

    const script = document.createElement("script")
    script.src =
      "https://js.hcaptcha.com/1/api.js?onload=__chargeHCaptchaReady&render=explicit&recaptchacompat=off"
    script.async = true
    script.defer = true
    script.dataset.hcaptchaScript = "true"
    script.onerror = () => {
      script.remove()
      reject(new Error("hCaptcha script failed"))
    }
    document.head.appendChild(script)
  }).catch((error: unknown) => {
    delete window.__chargeHCaptchaReady
    hcaptchaScriptPromise = null
    throw error
  })
  return hcaptchaScriptPromise
}

export const HCaptchaWidget = forwardRef<
  HCaptchaWidgetHandle,
  HCaptchaWidgetProps
>(function HCaptchaWidget({ siteKey, onVerified, onExpired, onError }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const widgetIdRef = useRef("")
  const callbacksRef = useRef({ onVerified, onExpired, onError })
  callbacksRef.current = { onVerified, onExpired, onError }
  const { resolvedTheme } = useTheme()
  const widgetTheme = resolvedTheme === "dark" ? "dark" : "light"

  useImperativeHandle(
    ref,
    () => ({
      reset: () => {
        if (widgetIdRef.current && window.hcaptcha)
          window.hcaptcha.reset(widgetIdRef.current)
      },
    }),
    []
  )

  useEffect(() => {
    let active = true

    async function renderWidget() {
      if (!siteKey || !containerRef.current) return
      try {
        await loadHCaptchaScript()
        if (!active || !window.hcaptcha || !containerRef.current) return
        if (widgetIdRef.current) window.hcaptcha.remove(widgetIdRef.current)
        widgetIdRef.current = window.hcaptcha.render(containerRef.current, {
          sitekey: siteKey,
          theme: widgetTheme,
          size: "normal",
          callback: (token: string) => callbacksRef.current.onVerified(token),
          "expired-callback": () => callbacksRef.current.onExpired(),
          "chalexpired-callback": () => callbacksRef.current.onExpired(),
          "error-callback": () => callbacksRef.current.onError(),
        })
      } catch {
        if (active) callbacksRef.current.onError()
      }
    }

    void renderWidget()
    return () => {
      active = false
      if (widgetIdRef.current && window.hcaptcha)
        window.hcaptcha.remove(widgetIdRef.current)
      widgetIdRef.current = ""
    }
  }, [siteKey, widgetTheme])

  return (
    <div
      ref={containerRef}
      className="grid min-h-20 w-full place-items-center overflow-x-auto"
      aria-label="人机验证"
    />
  )
})
