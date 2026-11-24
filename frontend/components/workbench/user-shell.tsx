"use client"
import { useMemo, type ComponentProps } from "react"
import dynamic from "next/dynamic"
import { AppShell } from "@/components/app-shell"
import { useDashboard } from "@/lib/dashboard-context"
import { useWatch } from "@/lib/watch-context"
import { useNotifications } from "@/lib/notification-context"
import { useOnline } from "@/lib/browser-state"
import { dashboardHref } from "@/lib/workbench"

const UsageGuideDialog = dynamic(
  () =>
    import("@/components/usage-guide-dialog").then(
      (module) => module.UsageGuideDialog
    ),
  { ssr: false }
)

export function UserWorkbenchShell(props: ComponentProps<typeof AppShell>) {
  const { snapshot, streamState } = useDashboard(),
    { rules } = useWatch(),
    { unreadCount } = useNotifications(),
    online = useOnline()
  const counts = useMemo(
    () => ({
      reminders: rules.filter(
        (rule) =>
          rule.mode === "temporary" && rule.enabled && !rule.completedAt
      ).length,
      notifications: unreadCount,
    }),
    [rules, unreadCount]
  )
  const searchItems = useMemo(
    () =>
      snapshot.piles.map((pile) => ({
        title: pile.name || pile.number,
        description: `${pile.number} · ${pile.address}`,
        href: dashboardHref(pile.id),
      })),
    [snapshot.piles]
  )
  const guideAction = useMemo(() => <UsageGuideDialog compact />, [])
  return (
    <AppShell
      {...props}
      connectionState={
        !online
          ? "offline"
          : streamState === "error"
            ? "reconnecting"
            : "connected"
      }
      counts={counts}
      searchItems={searchItems}
      guideAction={guideAction}
    />
  )
}
