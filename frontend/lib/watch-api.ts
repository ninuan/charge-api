import type {
  NotificationPreference,
  NotificationPreferenceUpdateRequest,
  WatchOverview,
  WatchRule,
  WatchRuleCreateRequest,
  WatchRuleUpdateRequest,
} from "@/lib/api/generated"
import { request } from "@/lib/http"

const jsonHeaders = { "Content-Type": "application/json" }

export const watchApi = {
  overview: () =>
    request<WatchOverview>("/api/watch-overview", {}, "加载关注额度失败"),
  rules: () => request<WatchRule[]>("/api/watch-rules", {}, "加载关注列表失败"),
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
      "创建关注规则失败"
    ),
  updateRule: (ruleId: string, payload: WatchRuleUpdateRequest) =>
    request<WatchRule>(
      `/api/watch-rules/${encodeURIComponent(ruleId)}`,
      { method: "PATCH", headers: jsonHeaders, body: JSON.stringify(payload) },
      "更新关注规则失败"
    ),
  deleteRule: (ruleId: string) =>
    request<void>(
      `/api/watch-rules/${encodeURIComponent(ruleId)}`,
      { method: "DELETE" },
      "删除关注规则失败"
    ),
  updatePreference: (payload: NotificationPreferenceUpdateRequest) =>
    request<NotificationPreference>(
      "/api/notification-preferences",
      { method: "PATCH", headers: jsonHeaders, body: JSON.stringify(payload) },
      "更新免打扰设置失败"
    ),
}
