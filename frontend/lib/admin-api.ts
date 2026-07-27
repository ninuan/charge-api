import type {
  AdminHealth,
  AdminStats,
  AdminUserDetail,
  AdminUserListQuery,
  AdminUserPage,
  AuditPage,
  CurrentUser,
  InviteCode,
  InviteCodePage,
  OperationsStatus,
  RegistrationSettings,
  SystemException,
  TemporaryPasswordResponse,
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
  userDetail: (id: string) =>
    request<AdminUserDetail>(
      `/api/admin/users/${id}/detail`,
      {},
      "加载用户详情失败"
    ),
  refreshUser: (id: string) =>
    request<void>(
      `/api/admin/users/${id}/refresh`,
      { method: "POST" },
      "刷新用户设备失败"
    ),
  resetUserPassword: (id: string) =>
    request<TemporaryPasswordResponse>(
      `/api/admin/users/${id}/reset-password`,
      { method: "POST" },
      "重置用户密码失败"
    ),
  incidents: (filters?: { status?: string; type?: string; level?: string }) =>
    request<SystemException[]>(
      `/api/admin/incidents?${new URLSearchParams({
        status: filters?.status ?? "",
        type: filters?.type ?? "",
        level: filters?.level ?? "",
      })}`,
      {},
      "加载异常列表失败"
    ),
  updateIncident: (
    id: string,
    payload: { status: SystemException["status"]; note: string }
  ) =>
    request<SystemException>(
      `/api/admin/incidents/${id}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      },
      "更新异常状态失败"
    ),
  audit: (page = 1, pageSize = 20) =>
    request<AuditPage>(
      `/api/admin/audit?${new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
      })}`,
      {},
      "加载管理日志失败"
    ),
  operations: () =>
    request<OperationsStatus>("/api/admin/operations", {}, "加载运维信息失败"),
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
