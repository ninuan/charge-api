import type {
  DeletedCount,
  Notification,
  NotificationPage,
  NotificationPreference,
  NotificationPreferenceUpdateRequest,
  NotificationStatusFilter,
  UpdatedCount,
  WatchOverview,
  WatchRule,
  WatchRuleCreateRequest,
  WatchRuleUpdateRequest,
} from "@/lib/api/generated"
import { request } from "@/lib/http"

const jsonHeaders = { "Content-Type": "application/json" }

export const watchApi = {
  overview: () =>
    request<WatchOverview>("/api/watch-overview", {}, "加载空闲提醒额度失败"),
  rules: () =>
    request<WatchRule[]>("/api/watch-rules", {}, "加载空闲提醒列表失败"),
  preference: () =>
    request<NotificationPreference>(
      "/api/notification-preferences",
      {},
      "加载免打扰设置失败"
    ),
  createRule: (payload: WatchRuleCreateRequest) =>
    request<WatchRule>(
      "/api/watch-rules",
      { method: "POST", headers: jsonHeaders, body: JSON.stringify(payload) },
      "创建空闲提醒失败"
    ),
  updateRule: (ruleId: string, payload: WatchRuleUpdateRequest) =>
    request<WatchRule>(
      `/api/watch-rules/${encodeURIComponent(ruleId)}`,
      { method: "PATCH", headers: jsonHeaders, body: JSON.stringify(payload) },
      "更新空闲提醒失败"
    ),
  deleteRule: (ruleId: string) =>
    request<void>(
      `/api/watch-rules/${encodeURIComponent(ruleId)}`,
      { method: "DELETE" },
      "删除空闲提醒失败"
    ),
  updatePreference: (payload: NotificationPreferenceUpdateRequest) =>
    request<NotificationPreference>(
      "/api/notification-preferences",
      { method: "PATCH", headers: jsonHeaders, body: JSON.stringify(payload) },
      "更新免打扰设置失败"
    ),
  notifications: ({
    status = "all",
    cursor,
    limit = 20,
  }: {
    status?: NotificationStatusFilter
    cursor?: string
    limit?: number
  } = {}) => {
    const query = new URLSearchParams({ status, limit: String(limit) })
    if (cursor) query.set("cursor", cursor)
    return request<NotificationPage>(
      `/api/notifications?${query}`,
      {},
      "加载通知失败，请稍后重试"
    )
  },
  markNotificationRead: (notificationId: string) =>
    request<Notification>(
      `/api/notifications/${encodeURIComponent(notificationId)}/read`,
      { method: "POST" },
      "标记通知已读失败，请稍后重试"
    ),
  markAllNotificationsRead: () =>
    request<UpdatedCount>(
      "/api/notifications/read-all",
      { method: "POST" },
      "全部标记已读失败，请稍后重试"
    ),
  clearResolvedNotifications: () =>
    request<DeletedCount>(
      "/api/notifications/resolved",
      { method: "DELETE" },
      "清理已解决通知失败，请稍后重试"
    ),
}
