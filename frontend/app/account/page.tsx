"use client"

import { ArrowLeftIcon, ShieldCheckIcon } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"

import { AccountSecurityPanel } from "@/components/account-security-panel"
import { AppShell } from "@/components/app-shell"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth-context"

export default function AccountPage() {
  const router = useRouter()
  const { currentUser, fetchMe } = useAuth()
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let active = true
    async function authorize() {
      const user = currentUser ?? await fetchMe()
      if (!user) return router.replace("/login")
      if (user.role === "admin") return router.replace("/admin")
      if (active) setReady(true)
    }
    void authorize()
    return () => { active = false }
  }, [currentUser, fetchMe, router])

  if (!ready) return null

  return <AppShell title="账户中心" description="管理密码、当前登录会话和其他设备访问。" actions={<Button variant="outline" onClick={() => router.push("/dashboard")}><ArrowLeftIcon />返回看板</Button>}><section className="mb-6 flex items-center gap-3 rounded-xl border bg-background p-5"><span className="grid size-10 place-items-center rounded-lg bg-muted"><ShieldCheckIcon className="size-5" /></span><div><h2 className="font-semibold">{currentUser?.username}</h2><p className="mt-1 text-sm text-muted-foreground">普通用户 · 登录会话最长保留 7 天</p></div></section><AccountSecurityPanel /></AppShell>
}
