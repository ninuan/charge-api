"use client"

import { useCallback, useEffect, useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"

import { useAuth } from "@/lib/auth-context"
import { normalizeLegacyHash, resolveHomeRoute } from "@/lib/routing"

export default function Home() {
  const router = useRouter()
  const { fetchMe } = useAuth()
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  const retry = useCallback(() => {
    setError(null)
    setAttempt((value) => value + 1)
  }, [])

  useEffect(() => {
    const legacyPath = normalizeLegacyHash(window.location.hash)
    if (legacyPath) {
      router.replace(legacyPath)
      return
    }
    let active = true
    void fetchMe()
      .then((user) => {
        if (!active) return
        router.replace(resolveHomeRoute(user?.role ?? null))
      })
      .catch((reason) => {
        if (!active) return
        setError(
          reason instanceof Error
            ? reason.message
            : "暂时无法连接服务，请稍后重试。"
        )
      })
    return () => {
      active = false
    }
  }, [attempt, fetchMe, router])

  if (error) {
    return (
      <main className="grid min-h-dvh place-items-center bg-muted/30 p-6">
        <section className="grid w-full max-w-sm gap-4 rounded-xl border bg-background p-6 text-center shadow-xs">
          <div className="grid gap-1">
            <h1 className="text-base font-semibold text-foreground">
              暂时无法进入 Charge Console
            </h1>
            <p className="text-sm text-muted-foreground">{error}</p>
          </div>
          <div className="flex items-center justify-center gap-3">
            <button
              type="button"
              onClick={retry}
              className="inline-flex h-9 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
            >
              重试
            </button>
            <Link
              href="/login/"
              className="inline-flex h-9 items-center justify-center rounded-lg border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:bg-muted"
            >
              前往登录
            </Link>
          </div>
        </section>
      </main>
    )
  }

  return (
    <div className="grid min-h-dvh place-items-center bg-muted/30 p-6 text-sm text-muted-foreground">
      正在进入 Charge Console…
    </div>
  )
}
