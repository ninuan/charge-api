import type { AdminHealth, AdminStats, AdminUserListQuery, AdminUserPage, CurrentUser, InviteCode, RegistrationSettings, UserRole } from "@/lib/types"

async function adminRequest<T>(path: string, init: RequestInit = {}, fallback: string): Promise<T> {
  const response = await fetch(path, { credentials: "include", ...init })
  if (!response.ok && response.status !== 204) { const body = (await response.json().catch(() => ({ error: fallback }))) as { error?: string }; throw new Error(body.error ?? fallback) }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>
}

export const adminApi = {
  stats: () => adminRequest<AdminStats>("/api/admin/stats", {}, "加载运营统计失败"),
  health: () => adminRequest<AdminHealth>("/api/admin/health", {}, "加载系统状态失败"),
  settings: () => adminRequest<RegistrationSettings>("/api/admin/settings", {}, "加载系统策略失败"),
  invites: () => adminRequest<InviteCode[]>("/api/admin/invites", {}, "加载邀请码失败"),
  users: (query: AdminUserListQuery) => adminRequest<AdminUserPage>(`/api/admin/users?${new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize), search: query.search, account: query.account, credential: query.credential, health: query.health })}`, {}, "加载用户列表失败"),
  createUser: (payload: { username: string; password: string; role: UserRole }) => adminRequest<CurrentUser>("/api/admin/users", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) }, "创建用户失败"),
  updateUser: (id: string, payload: { enabled?: boolean; deviceLimit?: number; refreshEnabled?: boolean; password?: string }) => adminRequest<CurrentUser>(`/api/admin/users/${id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) }, "更新用户失败"),
  removeUser: (id: string) => adminRequest<void>(`/api/admin/users/${id}`, { method: "DELETE" }, "删除用户失败"),
  saveSettings: (payload: RegistrationSettings) => adminRequest<RegistrationSettings>("/api/admin/settings", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) }, "保存设置失败"),
  createInvite: () => adminRequest<InviteCode>("/api/admin/invites", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({}) }, "创建邀请码失败"),
  removeInvite: (id: string) => adminRequest<void>(`/api/admin/invites/${id}`, { method: "DELETE" }, "删除邀请码失败"),
}
