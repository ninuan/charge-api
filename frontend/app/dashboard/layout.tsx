import type { ReactNode } from "react"

import { DashboardProvider } from "@/lib/dashboard-context"

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return <DashboardProvider>{children}</DashboardProvider>
}
