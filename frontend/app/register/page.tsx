"use client"

import { useRouter } from "next/navigation"

import { AuthForm } from "@/components/auth-form"
import { AuthPageShell } from "@/components/auth-page-shell"

export default function RegisterPage() {
  const router = useRouter()
  return <AuthPageShell mode="register"><AuthForm mode="register" onSuccess={(path) => router.replace(path)} /></AuthPageShell>
}
