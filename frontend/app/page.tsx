"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"

import { useAuth } from "@/lib/auth-context"
import { normalizeLegacyHash, resolveHomeRoute } from "@/lib/routing"

export default function Home() {
  const router = useRouter()
  const { fetchMe } = useAuth()

  useEffect(() => {
    const legacyPath = normalizeLegacyHash(window.location.hash)
    if (legacyPath) {
      router.replace(legacyPath)
      return
    }
    void fetchMe().then((user) => router.replace(resolveHomeRoute(user?.role ?? null)))
  }, [fetchMe, router])

  return <div className="grid min-h-dvh place-items-center bg-muted/30 p-6 text-sm text-muted-foreground">正在进入 Charge Console…</div>
}
