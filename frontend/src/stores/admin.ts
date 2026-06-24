import { computed, ref } from "vue";
import { defineStore } from "pinia";
import type { AdminStats, AdminUserSummary, CurrentUser, DashboardCounters, InviteCode, RefreshInfo, RegistrationSettings, SystemException, TrafficStats, UserRole } from "@/types/dashboard";

type NullableAdminUserSummary = Omit<AdminUserSummary, "dashboard" | "deviceIds" | "lastRefresh" | "stats"> & {
  dashboard?: DashboardCounters | null;
  deviceIds?: string[] | null;
  lastRefresh?: RefreshInfo | null;
  stats?: TrafficStats | null;
};

type NullableAdminStats = {
  users?: NullableAdminUserSummary[] | null;
  hourly?: AdminStats["hourly"] | null;
  daily?: AdminStats["daily"] | null;
  exceptions?: SystemException[] | null;
};

const emptyStats: TrafficStats = {
  totalRequests: 0,
  refreshRequests: 0,
  remoteFetches: 0,
  cachedRefreshes: 0,
  failedRequests: 0,
  authFailures: 0
};

const emptyDashboard: DashboardCounters = {
  pileCount: 0,
  portCount: 0,
  inUsePortCount: 0,
  idlePortCount: 0,
  offlinePorts: 0
};

const emptyRefresh: RefreshInfo = {
  minIntervalSeconds: 30,
  attemptedDevices: 0,
  successfulDevices: 0,
  failedDevices: 0,
  skippedDevices: 0,
  cached: false,
  partial: false
};

function arrayOrEmpty<T>(value?: T[] | null) {
  return Array.isArray(value) ? value : [];
}

function normalizeSummary(summary: NullableAdminUserSummary): AdminUserSummary {
  return {
    ...summary,
    stats: summary.stats ?? emptyStats,
    dashboard: summary.dashboard ?? emptyDashboard,
    deviceIds: arrayOrEmpty(summary.deviceIds),
    lastRefresh: summary.lastRefresh ?? emptyRefresh
  };
}

export const useAdminStore = defineStore("admin", () => {
  const users = ref<AdminUserSummary[]>([]);
  const loading = ref(false);
  const settings = ref<RegistrationSettings | null>(null);
  const invites = ref<InviteCode[]>([]);
  const exceptions = ref<SystemException[]>([]);
  const hourly = ref<AdminStats["hourly"]>([]);
  const daily = ref<AdminStats["daily"]>([]);

  const totals = computed(() => users.value.reduce((result, summary) => {
    result.users += 1;
    result.enabledUsers += summary.user.enabled ? 1 : 0;
    result.totalRequests += summary.stats.totalRequests;
    result.refreshRequests += summary.stats.refreshRequests;
    result.remoteFetches += summary.stats.remoteFetches;
    result.cachedRefreshes += summary.stats.cachedRefreshes;
    result.failedRequests += summary.stats.failedRequests;
    result.authFailures += summary.stats.authFailures;
    result.devices += arrayOrEmpty(summary.deviceIds).length;
    return result;
  }, {
    users: 0,
    enabledUsers: 0,
    totalRequests: 0,
    refreshRequests: 0,
    remoteFetches: 0,
    cachedRefreshes: 0,
    failedRequests: 0,
    authFailures: 0,
    devices: 0
  }));

  async function load() {
    loading.value = true;
    try {
      const res = await fetch("/api/admin/stats", { credentials: "include" });
      if (!res.ok) throw new Error((await res.json()).error ?? "加载用户失败");
      const data = await res.json() as NullableAdminStats;
      users.value = arrayOrEmpty(data.users).map(normalizeSummary);
      exceptions.value = arrayOrEmpty(data.exceptions);
      hourly.value = arrayOrEmpty(data.hourly);
      daily.value = arrayOrEmpty(data.daily);
      const [settingsRes, invitesRes] = await Promise.all([
        fetch("/api/admin/settings", { credentials: "include" }),
        fetch("/api/admin/invites", { credentials: "include" })
      ]);
      if (settingsRes.ok) settings.value = await settingsRes.json();
      if (invitesRes.ok) invites.value = arrayOrEmpty(await invitesRes.json() as InviteCode[] | null);
    } finally {
      loading.value = false;
    }
  }

  async function create(payload: { username: string; password: string; role: UserRole }) {
    const res = await fetch("/api/admin/users", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) throw new Error((await res.json()).error ?? "创建用户失败");
    await load();
  }

  async function setEnabled(user: CurrentUser, enabled: boolean) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled })
    });
    if (!res.ok) throw new Error((await res.json()).error ?? "更新用户失败");
    await load();
  }

  async function updateUser(user: CurrentUser, payload: { deviceLimit?: number; refreshEnabled?: boolean; password?: string }) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "PATCH", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) throw new Error((await res.json()).error ?? "更新用户失败");
    await load();
  }

  async function saveSettings(value: RegistrationSettings) {
    const res = await fetch("/api/admin/settings", {
      method: "PUT", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value)
    });
    if (!res.ok) throw new Error((await res.json()).error ?? "保存设置失败");
    settings.value = await res.json();
  }

  async function createInvite() {
    const bytes = new Uint8Array(8);
    crypto.getRandomValues(bytes);
    const code = `CHG-${Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("").toUpperCase()}`;
    const res = await fetch("/api/admin/invites", {
      method: "POST", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code })
    });
    if (!res.ok) throw new Error((await res.json()).error ?? "创建邀请码失败");
    await load();
  }

  async function removeInvite(id: string) {
    const res = await fetch(`/api/admin/invites/${id}`, { method: "DELETE", credentials: "include" });
    if (!res.ok && res.status !== 204) throw new Error((await res.json()).error ?? "删除邀请码失败");
    await load();
  }

  async function remove(user: CurrentUser) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) throw new Error((await res.json()).error ?? "删除用户失败");
    await load();
  }

  return { users, loading, totals, settings, invites, exceptions, hourly, daily, load, create, setEnabled, updateUser, saveSettings, createInvite, removeInvite, remove };
});
