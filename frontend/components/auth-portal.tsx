"use client"

import { usePathname, useRouter } from "next/navigation"

import { AuthForm } from "@/components/auth-form"
import { AuthPageShell } from "@/components/auth-page-shell"

export function AuthPortal() {
  const pathname = usePathname()
  const router = useRouter()
  const mode = pathname.startsWith("/register") ? "register" : "login"

  return <AuthPageShell mode={mode}><AuthForm mode={mode} onSuccess={(path) => router.replace(path)} /></AuthPageShell>
}
