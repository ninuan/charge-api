"use client"

import {
  LogOutIcon,
  MenuIcon,
  ShieldCheckIcon,
  UserRoundIcon,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { type ReactNode, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/lib/auth-context"

export function AppShell({
  compact = false,
  title,
  description,
  actions,
  children,
}: {
  compact?: boolean
  title: string
  description: string
  actions?: ReactNode
  children: ReactNode
}) {
  const router = useRouter()
  const { currentUser, isAdmin, logout, ready } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)

  async function handleLogout() {
    await logout()
    router.replace("/login")
  }

  const identity = !ready ? (
    <div
      className="flex min-w-0 items-center gap-2 px-1 py-1"
      data-testid="identity-skeleton"
      aria-label="正在加载账户信息"
    >
      <Skeleton className="size-8 shrink-0 rounded-full" />
      <span className="flex min-w-0 flex-col gap-1.5">
        <Skeleton className="h-3.5 w-20" />
        <Skeleton className="h-3 w-14" />
      </span>
    </div>
  ) : (
    <Link
      href={isAdmin ? "/admin" : "/account"}
      aria-label="进入账户中心"
      className="flex min-w-0 items-center gap-2 rounded-lg px-1 py-1 transition-colors hover:bg-muted"
    >
      <span className="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold">
        {currentUser?.username.slice(0, 1).toUpperCase()}
      </span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-medium">
          {currentUser?.username}
        </span>
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          {isAdmin ? (
            <ShieldCheckIcon className="size-3" />
          ) : (
            <UserRoundIcon className="size-3" />
          )}
          {isAdmin ? "管理员" : "普通用户"}
        </span>
      </span>
    </Link>
  )

  const heading = compact ? (
    <section className="mb-4 flex flex-col gap-1 border-b pb-4 sm:flex-row sm:items-baseline sm:justify-between sm:gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="max-w-2xl text-xs leading-5 text-muted-foreground">
        {description}
      </p>
    </section>
  ) : (
    <section className="mb-5 border-b pb-5">
      <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
        {title}
      </h1>
      <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
        {description}
      </p>
    </section>
  )

  return (
    <div className="min-h-dvh bg-muted/35 text-foreground">
      <a
        className="sr-only focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2"
        href="#main-content"
      >
        跳到主要内容
      </a>
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-sm font-bold text-primary-foreground">
              C
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold tracking-tight">
                Charge Console
              </p>
              <p className="truncate text-xs text-muted-foreground">
                充电设施运营中心
              </p>
            </div>
          </div>
          <div className="hidden items-center gap-3 md:flex">
            {actions}
            {identity}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void handleLogout()}
            >
              <LogOutIcon />
              退出
            </Button>
          </div>
          <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
            <SheetTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="md:hidden"
                  aria-label="打开菜单"
                />
              }
            >
              <MenuIcon />
            </SheetTrigger>
            <SheetContent
              side="right"
              className="w-[calc(100vw-1rem)] max-w-[22rem] overflow-y-auto p-5"
            >
              <SheetTitle className="pr-8">账户与操作</SheetTitle>
              <div className="mt-6 grid gap-4">
                {identity}
                <div className="grid gap-2">{actions}</div>
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => void handleLogout()}
                >
                  <LogOutIcon />
                  退出登录
                </Button>
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </header>
      <main
        id="main-content"
        className="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8 lg:py-6"
      >
        {heading}
        {children}
      </main>
    </div>
  )
}
