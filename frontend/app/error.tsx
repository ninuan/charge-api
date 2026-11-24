"use client"

import Link from "next/link"

export default function AppError({
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  return (
    <main className="grid min-h-dvh place-items-center bg-muted/30 p-6">
      <section className="grid w-full max-w-md gap-4 rounded-xl border bg-background p-6 text-center shadow-xs">
        <div className="grid gap-1.5">
          <h1 className="text-base font-semibold text-foreground">
            页面加载遇到异常
          </h1>
          <p className="text-sm text-muted-foreground">
            请尝试重新加载当前视图，或返回首页继续使用。
          </p>
        </div>
        <div className="flex items-center justify-center gap-3">
          <button
            type="button"
            onClick={reset}
            className="inline-flex h-9 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            重试
          </button>
          <Link
            href="/"
            className="inline-flex h-9 items-center justify-center rounded-lg border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:bg-muted"
          >
            返回首页
          </Link>
        </div>
      </section>
    </main>
  )
}
