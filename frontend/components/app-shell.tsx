"use client"

import { LogOutIcon, MenuIcon, ShieldCheckIcon, UserRoundIcon } from "lucide-react"
import { useRouter } from "next/navigation"
import { type ReactNode, useState } from "react"

import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet"
import { useAuth } from "@/lib/auth-context"

export function AppShell({ title, description, actions, children }: { title: string; description: string; actions?: ReactNode; children: ReactNode }) {
  const router = useRouter()
  const { currentUser, isAdmin, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)

  async function handleLogout() {
    await logout()
    router.replace("/login")
  }

  const identity = <div className="flex min-w-0 items-center gap-2"><span className="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold">{currentUser?.username.slice(0, 1).toUpperCase()}</span><span className="min-w-0"><span className="block truncate text-sm font-medium">{currentUser?.username}</span><span className="flex items-center gap-1 text-xs text-muted-foreground">{isAdmin ? <ShieldCheckIcon className="size-3" /> : <UserRoundIcon className="size-3" />}{isAdmin ? "管理员" : "普通用户"}</span></span></div>

  return (
    <div className="min-h-dvh bg-muted/35 text-foreground">
      <a className="sr-only focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2" href="#main-content">跳到主要内容</a>
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
          <div className="flex min-w-0 items-center gap-3"><span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-sm font-bold text-primary-foreground">C</span><div className="min-w-0"><p className="truncate text-sm font-semibold tracking-tight">Charge Console</p><p className="truncate text-xs text-muted-foreground">充电设施运营中心</p></div></div>
          <div className="hidden items-center gap-3 md:flex">{actions}{identity}<Button variant="ghost" size="sm" onClick={() => void handleLogout()}><LogOutIcon />退出</Button></div>
          <Sheet open={menuOpen} onOpenChange={setMenuOpen}><SheetTrigger render={<Button variant="ghost" size="icon" className="md:hidden" aria-label="打开菜单" />}><MenuIcon /></SheetTrigger><SheetContent side="right" className="w-80"><SheetTitle>账户与操作</SheetTitle><div className="mt-6 grid gap-4">{identity}<div className="grid gap-2">{actions}</div><Button variant="outline" onClick={() => void handleLogout()}><LogOutIcon />退出登录</Button></div></SheetContent></Sheet>
        </div>
      </header>
      <main id="main-content" className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8 lg:py-10"><section className="mb-8 flex flex-col justify-between gap-5 border-b pb-7 md:flex-row md:items-end"><div><p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">Live infrastructure</p><h1 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">{title}</h1><p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{description}</p></div><div className="hidden md:flex">{actions}</div></section>{children}</main>
    </div>
  )
}
