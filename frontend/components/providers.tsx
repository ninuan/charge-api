"use client"

import dynamic from "next/dynamic"
import { type ReactNode } from "react"

import { ThemeProvider } from "@/components/theme-provider"
import { AuthProvider } from "@/lib/auth-context"
import { MotionPreference } from "@/lib/motion-preference"

const Toaster = dynamic(
  () => import("@/components/ui/sonner").then((module) => module.Toaster),
  { ssr: false }
)

export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <MotionPreference />
      <AuthProvider>{children}</AuthProvider>
      <Toaster />
    </ThemeProvider>
  )
}
