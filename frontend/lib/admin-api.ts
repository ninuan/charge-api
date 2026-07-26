import type {
  AdminHealth,
  AdminStats,
  AdminUserListQuery,
  AdminUserPage,
  CurrentUser,
  InviteCode,
  InviteCodePage,
  RegistrationSettings,
  UserRole,
} from "@/lib/types"
import { request } from "@/lib/http"

export const adminApi = {
  stats: () => request<AdminStats>("/api/admin/stats", {}, "加载运营统计失败"),
  health: () =>
    request<AdminHealth>("/api/admin/health", {}, "加载系统状态失败"),
  settings: () =>
    request<RegistrationSettings>(
      "/api/admin/settings",
      {},
      "加载系统策略失败"
    ),
  invites: ({ page, pageSize }: { page: number; pageSize: number }) =>
    request<InviteCodePage>(
      `/api/admin/invites?${new URLSearchParams({ page: String(page), pageSize: String(pageSize) })}`,
      {},
      "加载邀请码失败"
    ),
  users: (query: AdminUserListQuery) =>
    request<AdminUserPage>(
      `/api/admin/users?${new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize), search: query.search, account: query.account, credential: query.credential, health: query.health })}`,
      {},
      "加载用户列表失败"
    ),
  createUser: (payload: {
    username: string
    password: string
    role: UserRole
  }) =>
    request<CurrentUser>(
      "/api/admin/users",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      },
      "创建用户失败"
    ),
  updateUser: (
    id: string,
    payload: {
      enabled?: boolean
      deviceLimit?: number
      refreshEnabled?: boolean
      password?: string
    }
  ) =>
    request<CurrentUser>(
      `/api/admin/users/${id}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      },
      "更新用户失败"
    ),
  removeUser: (id: string) =>
    request<void>(
      `/api/admin/users/${id}`,
      { method: "DELETE" },
      "删除用户失败"
    ),
  saveSettings: (payload: RegistrationSettings) =>
    request<RegistrationSettings>(
      "/api/admin/settings",
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      },
      "保存设置失败"
    ),
  createInvite: () =>
    request<InviteCode>(
      "/api/admin/invites",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      },
      "创建邀请码失败"
    ),
  removeInvite: (id: string) =>
    request<void>(
      `/api/admin/invites/${id}`,
      { method: "DELETE" },
      "删除邀请码失败"
    ),
}
