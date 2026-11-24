import type {
  Notification,
  NotificationDeliverySummary,
  NotificationType,
} from "@/lib/api/generated"

export type NotificationActionLifecycle = "informational" | "action_required"

export function notificationActionLifecycle(
  type: NotificationType
): NotificationActionLifecycle {
  switch (type) {
    case "credential_expired":
    case "pile_offline":
      return "action_required"
    case "pile_available":
    case "pile_recovered":
    default:
      return "informational"
  }
}

export function notificationRequiresAction(type: NotificationType) {
  return notificationActionLifecycle(type) === "action_required"
}

export function notificationPresentation(notification: Notification) {
  switch (notification.type) {
    case "pile_available":
      return {
        title: notification.portId
          ? `${notification.portId} 号充电口空闲了`
          : "有空闲充电口",
        message: notification.message,
      }
    case "credential_expired":
      return {
        title: "需要重新登录",
        message: "个人 Cookie 已过期，请前往账号页更新后继续查看和接收提醒。",
      }
    case "pile_offline":
      return {
        title: "充电桩无法连接",
        message: notification.message,
      }
    case "pile_recovered":
      return {
        title: "充电桩恢复连接",
        message: notification.message,
      }
    default:
      return {
        title: notification.title || "站内通知",
        message: notification.message || "",
      }
  }
}

export type DeliveryPresentationState =
  | "queued"
  | "submitted"
  | "processed"
  | "suppressed"
  | "failed"
  | "cancelled"
  | "unknown"

export function deliveryPresentationState(
  delivery: NotificationDeliverySummary
): DeliveryPresentationState {
  if (delivery.userState) return delivery.userState

  switch (delivery.status) {
    case "pending":
    case "sending":
    case "retry_wait":
      return "queued"
    case "accepted":
      return "submitted"
    case "provider_succeeded":
      return "processed"
    case "suppressed":
      return "suppressed"
    case "failed":
      return "failed"
    case "cancelled":
      return "cancelled"
    case "uncertain":
      return delivery.acceptedAt ? "submitted" : "unknown"
  }
}

export function deliveryPresentationMessage(
  delivery: NotificationDeliverySummary
) {
  switch (deliveryPresentationState(delivery)) {
    case "queued":
      return "等待发送"
    case "submitted":
      return "已发送，请检查手机"
    case "processed":
      return "已发送，请检查手机"
    case "suppressed":
      return "免打扰时段未发送"
    case "failed":
      return "发送失败"
    case "cancelled":
      return "已取消"
    case "unknown":
      return "发送结果暂时无法确认"
  }
}
