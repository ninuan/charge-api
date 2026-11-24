import Link from "next/link"

export default function NotFound() {
  return (
    <main className="grid min-h-dvh place-items-center bg-muted/30 p-6">
      <section className="grid w-full max-w-md gap-4 rounded-xl border bg-background p-6 text-center shadow-xs">
        <div className="grid gap-1.5">
          <p className="text-xs font-medium tracking-wide text-muted-foreground">
            404
          </p>
          <h1 className="text-base font-semibold text-foreground">
            页面不存在或已迁移
          </h1>
          <p className="text-sm text-muted-foreground">
            请返回首页或直接进入充电桩工作台。
          </p>
        </div>
        <div className="flex items-center justify-center gap-3">
          <Link
            href="/dashboard/"
            className="inline-flex h-9 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            进入工作台
          </Link>
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
