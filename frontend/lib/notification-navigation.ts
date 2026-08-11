import type { Notification } from "@/lib/api/generated"

export const notificationNavigateEvent = "charge:notification-navigate"

export function navigateToNotification(notification: Notification) {
  window.dispatchEvent(
    new CustomEvent<Notification>(notificationNavigateEvent, {
      detail: notification,
    })
  )
}
