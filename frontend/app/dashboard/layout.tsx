import type { ReactNode } from "react"

import { DashboardProvider } from "@/lib/dashboard-context"
import { NotificationProvider } from "@/lib/notification-context"
import { WatchProvider } from "@/lib/watch-context"
import { DashboardSession } from "@/components/workbench/dashboard-session"

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return (
    <DashboardProvider>
      <WatchProvider>
        <NotificationProvider>
          <DashboardSession>{children}</DashboardSession>
        </NotificationProvider>
      </WatchProvider>
    </DashboardProvider>
  )
}
