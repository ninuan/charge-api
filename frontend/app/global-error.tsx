"use client"

export default function GlobalError({
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  return (
    <html lang="zh-CN">
      <body className="grid min-h-dvh place-items-center bg-background p-6 text-foreground antialiased">
        <main className="grid w-full max-w-md gap-4 rounded-xl border p-6 text-center shadow-xs">
          <div className="grid gap-1.5">
            <h1 className="text-base font-semibold">应用出现意外错误</h1>
            <p className="text-sm opacity-75">
              你可以尝试重新加载页面恢复使用。
            </p>
          </div>
          <div className="flex items-center justify-center gap-3">
            <button
              type="button"
              onClick={reset}
              className="inline-flex h-9 items-center justify-center rounded-lg bg-foreground px-4 text-sm font-medium text-background"
            >
              重新加载
            </button>
          </div>
        </main>
      </body>
    </html>
  )
}
