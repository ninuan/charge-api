"use client"
import type { ComponentProps } from "react"
import { AppShell } from "@/components/app-shell"
import { UsageGuideDialog } from "@/components/usage-guide-dialog"
import { useDashboard } from "@/lib/dashboard-context"
import { useWatch } from "@/lib/watch-context"
import { useNotifications } from "@/lib/notification-context"
import { dashboardHref } from "@/lib/workbench"

export function UserWorkbenchShell(props: ComponentProps<typeof AppShell>) {
  const { snapshot } = useDashboard(),
    { rules } = useWatch(),
    { unreadCount } = useNotifications()
  return (
    <AppShell
      {...props}
      counts={{
        reminders: rules.filter(
          (rule) =>
            rule.mode === "temporary" && rule.enabled && !rule.completedAt
        ).length,
        notifications: unreadCount,
      }}
      searchItems={snapshot.piles.map((pile) => ({
        title: pile.name || pile.number,
        description: `${pile.number} · ${pile.address}`,
        href: dashboardHref(pile.id),
      }))}
      guideAction={<UsageGuideDialog />}
    />
  )
}
