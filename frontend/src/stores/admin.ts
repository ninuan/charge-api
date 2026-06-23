import { computed, ref } from "vue";
import { defineStore } from "pinia";
import type { AdminUserSummary, CurrentUser, UserRole } from "@/types/dashboard";

export const useAdminStore = defineStore("admin", () => {
  const users = ref<AdminUserSummary[]>([]);
  const loading = ref(false);

  const totals = computed(() => users.value.reduce((result, summary) => {
    result.users += 1;
    result.enabledUsers += summary.user.enabled ? 1 : 0;
    result.totalRequests += summary.stats.totalRequests;
    result.remoteFetches += summary.stats.remoteFetches;
    result.cachedRefreshes += summary.stats.cachedRefreshes;
    result.failedRequests += summary.stats.failedRequests;
    result.authFailures += summary.stats.authFailures;
    result.devices += summary.deviceIds.length;
    return result;
  }, {
    users: 0,
    enabledUsers: 0,
    totalRequests: 0,
    remoteFetches: 0,
    cachedRefreshes: 0,
    failedRequests: 0,
    authFailures: 0,
    devices: 0
  }));

  async function load() {
    loading.value = true;
    try {
      const res = await fetch("/api/admin/users", { credentials: "include" });
      if (!res.ok) throw new Error((await res.json()).error ?? "加载用户失败");
      users.value = await res.json() as AdminUserSummary[];
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

  async function remove(user: CurrentUser) {
    const res = await fetch(`/api/admin/users/${user.id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) throw new Error((await res.json()).error ?? "删除用户失败");
    await load();
  }

  return { users, loading, totals, load, create, setEnabled, remove };
});
