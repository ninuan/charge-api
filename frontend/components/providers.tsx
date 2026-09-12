"use client"

import { type ReactNode } from "react"

import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"
import { AuthProvider } from "@/lib/auth-context"
import { MotionPreference } from "@/lib/motion-preference"

export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <MotionPreference />
      <AuthProvider>{children}</AuthProvider>
      <Toaster />
    </ThemeProvider>
  )
}
