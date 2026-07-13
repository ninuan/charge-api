import { computed, ref } from "vue";
import { defineStore } from "pinia";
import type {
  AdminHealth,
  AdminUserListQuery,
  AdminUserPage,
  AdminOverview,
  AdminStats,
  AdminUserSummary,
  CredentialSummary,
  CurrentUser,
  DashboardCounters,
  InviteCode,
  RecoveryDiagnostic,
  RefreshInfo,
  RegistrationSettings,
  SystemException,
  TrafficStats,
  UserRole
} from "@/types/dashboard";

type NullableAdminUserSummary = Omit<AdminUserSummary, "credential" | "dashboard" | "deviceIds" | "lastRefresh" | "snapshotUpdatedAt" | "stats" | "recoveryDiagnostics"> & {
  credential?: CredentialSummary | null;
  dashboard?: DashboardCounters | null;
  deviceIds?: string[] | null;
  lastRefresh?: RefreshInfo | null;
  snapshotUpdatedAt?: string | null;
  stats?: TrafficStats | null;
  recoveryDiagnostics?: RecoveryDiagnostic[] | null;
};

type NullableAdminStats = {
  overview?: AdminOverview | null;
  users?: NullableAdminUserSummary[] | null;
  hourly?: AdminStats["hourly"] | null;
  daily?: AdminStats["daily"] | null;
  exceptions?: SystemException[] | null;
};

type NullableAdminUserPage = Omit<AdminUserPage, "items"> & {
  items?: NullableAdminUserSummary[] | null;
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

const emptyOverview: AdminOverview = {
  openIssues: 0,
  remoteSuccessRate: 0,
  activeUsers: 0,
  managedDevices: 0,
  offlinePorts: 0
};

const emptyCredential: CredentialSummary = {
  state: "unbound",
  bound: false,
  hasCredential: false
};

const defaultUserListQuery: AdminUserListQuery = {
  page: 1,
  pageSize: 15,
  search: "",
  account: "all",
  credential: "all",
  health: "all"
};

const emptyUserPage: AdminUserPage = {
  items: [],
  page: 1,
  pageSize: 15,
  total: 0,
  totalPages: 1
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
    credential: summary.credential ?? emptyCredential,
    recoveryDiagnostics: arrayOrEmpty(summary.recoveryDiagnostics),
    snapshotUpdatedAt: summary.snapshotUpdatedAt ?? summary.user.createdAt,
    lastRefresh: summary.lastRefresh ?? emptyRefresh
  };
}

function normalizeUserPage(page: NullableAdminUserPage, query: AdminUserListQuery): AdminUserPage {
  return {
    items: arrayOrEmpty(page.items).map(normalizeSummary),
    page: Math.max(1, page.page ?? query.page),
    pageSize: Math.max(1, page.pageSize ?? query.pageSize),
    total: Math.max(0, page.total ?? 0),
    totalPages: Math.max(1, page.totalPages ?? 1)
  };
}

async function responseError(res: Response, fallback: string) {
  const payload = await res.json().catch(() => ({ error: fallback })) as { error?: string };
  return payload.error ?? fallback;
}

export const useAdminStore = defineStore("admin", () => {
  const users = ref<AdminUserSummary[]>([]);
  const userPage = ref<AdminUserPage>({ ...emptyUserPage });
  const userQuery = ref<AdminUserListQuery>({ ...defaultUserListQuery });
  const settings = ref<RegistrationSettings | null>(null);
  const invites = ref<InviteCode[]>([]);
  const exceptions = ref<SystemException[]>([]);
  const hourly = ref<AdminStats["hourly"]>([]);
  const daily = ref<AdminStats["daily"]>([]);
  const overview = ref<AdminOverview>({ ...emptyOverview });
  const health = ref<AdminHealth | null>(null);
  const lastUpdatedAt = ref<string>();

  const statsLoading = ref(false);
  const healthLoading = ref(false);
  const settingsLoading = ref(false);
  const invitesLoading = ref(false);
  const usersLoading = ref(false);
  const usersPrefetching = ref(false);
  const statsError = ref("");
  const healthError = ref("");
  const settingsError = ref("");
  const invitesError = ref("");
  const usersError = ref("");
  const loading = computed(() =>
    statsLoading.value || healthLoading.value || settingsLoading.value || invitesLoading.value || usersLoading.value
  );
  const userPageCache = new Map<string, AdminUserPage>();
  const userPageRequests = new Map<string, Promise<AdminUserPage>>();

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

  async function loadStats() {
    statsLoading.value = true;
    statsError.value = "";
    try {
      const res = await fetch("/api/admin/stats", { credentials: "include" });
      if (!res.ok) throw new Error(await responseError(res, "加载运营统计失败"));
      const data = await res.json() as NullableAdminStats;
      overview.value = data.overview ?? { ...emptyOverview };
      users.value = arrayOrEmpty(data.users).map(normalizeSummary);
      exceptions.value = arrayOrEmpty(data.exceptions);
      hourly.value = arrayOrEmpty(data.hourly);
      daily.value = arrayOrEmpty(data.daily);
      lastUpdatedAt.value = new Date().toISOString();
    } catch (error) {
      statsError.value = (error as Error).message;
    } finally {
      statsLoading.value = false;
    }
  }

  function normalizeUserQuery(next: Partial<AdminUserListQuery> = {}) {
    const query = { ...userQuery.value, ...next };
    return {
      ...query,
      page: Math.max(1, Number.isFinite(query.page) ? Math.floor(query.page) : 1),
      pageSize: Math.min(100, Math.max(1, Number.isFinite(query.pageSize) ? Math.floor(query.pageSize) : defaultUserListQuery.pageSize)),
      search: query.search.trim()
    } as AdminUserListQuery;
  }

  function userPageKey(query: AdminUserListQuery) {
    return `${query.page}|${query.pageSize}|${query.search}|${query.account}|${query.credential}|${query.health}`;
  }

  function userPageURL(query: AdminUserListQuery) {
    const params = new URLSearchParams({
      page: String(query.page),
      pageSize: String(query.pageSize),
      search: query.search,
      account: query.account,
      credential: query.credential,
      health: query.health
    });
    return `/api/admin/users?${params.toString()}`;
  }

  async function requestUserPage(query: AdminUserListQuery) {
    const key = userPageKey(query);
    const pending = userPageRequests.get(key);
    if (pending) return pending;

    const request = (async () => {
      const res = await fetch(userPageURL(query), { credentials: "include" });
      if (!res.ok) throw new Error(await responseError(res, "加载用户列表失败"));
      const page = normalizeUserPage(await res.json() as NullableAdminUserPage, query);
      userPageCache.set(userPageKey({ ...query, page: page.page, pageSize: page.pageSize }), page);
      return page;
    })();
    userPageRequests.set(key, request);
    try {
      return await request;
    } finally {
      userPageRequests.delete(key);
    }
  }

  async function prefetchUserPage(page: AdminUserPage, query: AdminUserListQuery) {
    if (page.page >= page.totalPages) return;
    const nextQuery = { ...query, page: page.page + 1, pageSize: page.pageSize };
    const key = userPageKey(nextQuery);
    if (userPageCache.has(key) || userPageRequests.has(key)) return;
    usersPrefetching.value = true;
    try {
      await requestUserPage(nextQuery);
    } catch {
      // 预加载失败不影响当前页，用户翻页时仍会正常请求。
    } finally {
      usersPrefetching.value = false;
    }
  }

  async function loadUserPage(next: Partial<AdminUserListQuery> = {}, options: { force?: boolean } = {}) {
    const query = normalizeUserQuery(next);
    const key = userPageKey(query);
    userQuery.value = query;
    usersError.value = "";
    const cached = userPageCache.get(key);
    if (cached && !options.force) {
      userPage.value = cached;
      void prefetchUserPage(cached, query);
      return;
    }

    usersLoading.value = true;
    try {
      const page = await requestUserPage(query);
      userPage.value = page;
      userQuery.value = { ...query, page: page.page, pageSize: page.pageSize };
      void prefetchUserPage(page, userQuery.value);
    } catch (error) {
      usersError.value = (error as Error).message;
    } finally {
      usersLoading.value = false;
    }
  }

  function invalidateUserPages() {
    userPageCache.clear();
  }

  async function loadHealth() {
    healthLoading.value = true;
    healthError.value = "";
    try {
      const res = await fetch("/api/admin/health", { credentials: "include" });
      if (!res.ok) throw new Error(await responseError(res, "加载系统状态失败"));
      health.value = await res.json() as AdminHealth;
    } catch (error) {
      healthError.value = (error as Error).message;
    } finally {
      healthLoading.value = false;
    }
  }

  async function loadSettings() {
    settingsLoading.value = true;
    settingsError.value = "";
    try {
      const res = await fetch("/api/admin/settings", { credentials: "include" });
      if (!res.ok) throw new Error(await responseError(res, "加载系统策略失败"));
      settings.value = await res.json() as RegistrationSettings;
    } catch (error) {
      settingsError.value = (error as Error).message;
    } finally {
      settingsLoading.value = false;
    }
  }

  async function loadInvites() {
    invitesLoading.value = true;
    invitesError.value = "";
    try {
      const res = await fetch("/api/admin/invites", { credentials: "include" });
      if (!res.ok) throw new Error(await responseError(res, "加载邀请码失败"));
      invites.value = arrayOrEmpty(await res.json() as InviteCode[] | null);
    } catch (error) {
      invitesError.value = (error as Error).message;
    } finally {
      invitesLoading.value = false;
    }
  }

  async function loadOverview() {
    await Promise.allSettled([loadStats(), loadHealth()]);
  }

  async function load() {
    await Promise.allSettled([loadOverview(), loadSettings(), loadInvites()]);
  }

  async function create(payload: { username: string; password: string; role: UserRole }) {
    const res = await fetch("/api/admin/users", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) throw new Error(await responseError(res, "创建用户失败"));
    invalidateUserPages();
    await loadStats();
  }

  async function setEnabled(user: CurrentUser, enabled: boolean) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled })
    });
    if (!res.ok) throw new Error(await responseError(res, "更新用户失败"));
    invalidateUserPages();
    await loadStats();
  }

  async function updateUser(user: CurrentUser, payload: { deviceLimit?: number; refreshEnabled?: boolean; password?: string }) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "PATCH", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) throw new Error(await responseError(res, "更新用户失败"));
    invalidateUserPages();
    await loadStats();
  }

  async function saveSettings(value: RegistrationSettings) {
    const res = await fetch("/api/admin/settings", {
      method: "PUT", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(value)
    });
    if (!res.ok) throw new Error(await responseError(res, "保存设置失败"));
    settings.value = await res.json();
  }

  async function createInvite() {
    const res = await fetch("/api/admin/invites", {
      method: "POST", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({})
    });
    if (!res.ok) throw new Error(await responseError(res, "创建邀请码失败"));
    await loadInvites();
  }

  async function removeInvite(id: string) {
    const res = await fetch(`/api/admin/invites/${id}`, { method: "DELETE", credentials: "include" });
    if (!res.ok && res.status !== 204) throw new Error(await responseError(res, "删除邀请码失败"));
    await loadInvites();
  }

  async function remove(user: CurrentUser) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) throw new Error(await responseError(res, "删除用户失败"));
    invalidateUserPages();
    await loadStats();
  }

  return {
    users, userPage, userQuery, loading, totals, settings, invites, exceptions, hourly, daily, overview, health, lastUpdatedAt,
    statsLoading, healthLoading, settingsLoading, invitesLoading, usersLoading, usersPrefetching,
    statsError, healthError, settingsError, invitesError, usersError,
    load, loadStats, loadHealth, loadSettings, loadInvites, loadOverview, loadUserPage, invalidateUserPages,
    create, setEnabled, updateUser, saveSettings, createInvite, removeInvite, remove
  };
});
