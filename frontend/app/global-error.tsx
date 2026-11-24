"use client"

import "./globals.css"

export default function GlobalError({
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  return (
    <html lang="zh-CN">
      <head>
        <title>应用出现意外错误 - Charge Console</title>
      </head>
      <body
        className="grid min-h-dvh place-items-center bg-background p-6 text-foreground antialiased"
        style={{
          margin: 0,
          minHeight: "100dvh",
          display: "grid",
          placeItems: "center",
          padding: 24,
          fontFamily: "system-ui, -apple-system, sans-serif",
        }}
      >
        <main
          className="grid w-full max-w-md gap-4 rounded-xl border p-6 text-center shadow-xs"
          style={{
            width: "100%",
            maxWidth: 448,
            display: "grid",
            gap: 16,
            padding: 24,
            borderRadius: 12,
            border: "1px solid rgba(128,128,128,0.28)",
            textAlign: "center",
          }}
        >
          <div className="grid gap-1.5">
            <h1 className="text-base font-semibold">应用出现意外错误</h1>
            <p className="text-sm opacity-75">
              你可以先尝试恢复页面，或刷新整个页面重新加载资源。
            </p>
          </div>
          <div className="flex flex-wrap items-center justify-center gap-3">
            <button
              type="button"
              onClick={reset}
              className="inline-flex h-9 items-center justify-center rounded-lg border px-4 text-sm font-medium"
            >
              重试渲染
            </button>
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="inline-flex h-9 items-center justify-center rounded-lg bg-foreground px-4 text-sm font-medium text-background"
            >
              刷新页面
            </button>
          </div>
        </main>
      </body>
    </html>
  )
}
