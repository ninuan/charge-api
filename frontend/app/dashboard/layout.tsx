import type { ReactNode } from "react"

import { DashboardProvider } from "@/lib/dashboard-context"
import { NotificationProvider } from "@/lib/notification-context"
import { WatchProvider } from "@/lib/watch-context"

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return (
    <DashboardProvider>
      <WatchProvider>
        <NotificationProvider>{children}</NotificationProvider>
      </WatchProvider>
    </DashboardProvider>
  )
}
