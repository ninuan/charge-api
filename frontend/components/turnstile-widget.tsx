"use client"

import { forwardRef, useEffect, useImperativeHandle, useRef } from "react"
import { useTheme } from "next-themes"

type TurnstileApi = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string
  reset: (widgetId?: string) => void
  remove: (widgetId: string) => void
}

declare global {
  interface Window {
    turnstile?: TurnstileApi
  }
}

export type TurnstileWidgetHandle = { reset: () => void }

type TurnstileWidgetProps = {
  siteKey: string
  action: "login" | "register"
  onVerified: (token: string) => void
  onExpired: () => void
  onError: () => void
}

function loadTurnstileScript() {
  return new Promise<void>((resolve, reject) => {
    if (window.turnstile) return resolve()
    const existing = document.querySelector<HTMLScriptElement>(
      "script[data-turnstile-script]"
    )
    if (existing) {
      existing.addEventListener("load", () => resolve(), { once: true })
      existing.addEventListener(
        "error",
        () => reject(new Error("Turnstile script failed")),
        { once: true }
      )
      return
    }

    const script = document.createElement("script")
    script.src =
      "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit"
    script.async = true
    script.defer = true
    script.dataset.turnstileScript = "true"
    script.onload = () => resolve()
    script.onerror = () => reject(new Error("Turnstile script failed"))
    document.head.appendChild(script)
  })
}

export const TurnstileWidget = forwardRef<
  TurnstileWidgetHandle,
  TurnstileWidgetProps
>(function TurnstileWidget(
  { siteKey, action, onVerified, onExpired, onError },
  ref
) {
  const containerRef = useRef<HTMLDivElement>(null)
  const widgetIdRef = useRef("")
  const callbacksRef = useRef({ onVerified, onExpired, onError })
  callbacksRef.current = { onVerified, onExpired, onError }
  const { resolvedTheme } = useTheme()
  // 主题变化会触发 effect 重跑：旧组件被 remove 后按新主题重新渲染。
  const widgetTheme = resolvedTheme === "dark" ? "dark" : "light"

  useImperativeHandle(
    ref,
    () => ({
      reset: () => {
        if (widgetIdRef.current && window.turnstile)
          window.turnstile.reset(widgetIdRef.current)
      },
    }),
    []
  )

  useEffect(() => {
    let active = true

    async function renderWidget() {
      if (!siteKey || !containerRef.current) return
      try {
        await loadTurnstileScript()
        if (!active || !window.turnstile || !containerRef.current) return
        if (widgetIdRef.current) window.turnstile.remove(widgetIdRef.current)
        widgetIdRef.current = window.turnstile.render(containerRef.current, {
          sitekey: siteKey,
          action,
          theme: widgetTheme,
          size: "flexible",
          callback: (token: string) => callbacksRef.current.onVerified(token),
          "expired-callback": () => callbacksRef.current.onExpired(),
          "error-callback": () => callbacksRef.current.onError(),
        })
      } catch {
        if (active) callbacksRef.current.onError()
      }
    }

    void renderWidget()
    return () => {
      active = false
      if (widgetIdRef.current && window.turnstile)
        window.turnstile.remove(widgetIdRef.current)
      widgetIdRef.current = ""
    }
  }, [action, siteKey, widgetTheme])

  return (
    <div
      ref={containerRef}
      className="grid min-h-16 w-full place-items-center"
      aria-label="人机验证"
    />
  )
})
