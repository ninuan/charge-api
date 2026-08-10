import type { ReactNode } from "react"

import { DashboardProvider } from "@/lib/dashboard-context"
import { WatchProvider } from "@/lib/watch-context"

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return (
    <DashboardProvider>
      <WatchProvider>{children}</WatchProvider>
    </DashboardProvider>
  )
}
