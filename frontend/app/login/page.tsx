"use client"

import { useRouter } from "next/navigation"

import { AuthForm } from "@/components/auth-form"
import { AuthPageShell } from "@/components/auth-page-shell"

export default function LoginPage() {
  const router = useRouter()
  return <AuthPageShell mode="login"><AuthForm mode="login" onSuccess={(path) => router.replace(path)} /></AuthPageShell>
}
