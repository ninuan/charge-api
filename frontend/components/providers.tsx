"use client"

import { type ReactNode } from "react"

import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"
import { AuthProvider } from "@/lib/auth-context"
import { DashboardProvider } from "@/lib/dashboard-context"

export function Providers({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
      <AuthProvider>
        <DashboardProvider>{children}</DashboardProvider>
      </AuthProvider>
      <Toaster />
    </ThemeProvider>
  )
}
