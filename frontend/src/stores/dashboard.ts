import { defineStore } from "pinia";
import { computed, ref } from "vue";
import type { DashboardSnapshot, Pile } from "../types/dashboard";

const emptySnapshot: DashboardSnapshot = {
  piles: [],
  updatedAt: new Date().toISOString(),
  statistics: {
    pileCount: 0,
    portCount: 0,
    inUsePortCount: 0,
    idlePortCount: 0,
    offlinePorts: 0
  },
  refresh: {
    minIntervalSeconds: 30,
    attemptedDevices: 0,
    successfulDevices: 0,
    failedDevices: 0,
    skippedDevices: 0,
    cached: false,
    partial: false
  }
};

async function throwResponseError(res: Response, fallback: string): Promise<never> {
  if (res.status === 401) {
    throw new Error("登录已失效，请重新登录");
  }
  const err = await res.json().catch(() => ({ error: fallback }));
  throw new Error(err.error ?? fallback);
}

export const useDashboardStore = defineStore("dashboard", () => {
  const snapshot = ref<DashboardSnapshot>(emptySnapshot);
  const loading = ref(false);

  const piles = computed(() => snapshot.value.piles);
  const stats = computed(() => snapshot.value.statistics);
  const refresh = computed(() => snapshot.value.refresh);

  function setSnapshot(nextSnapshot: DashboardSnapshot) {
    snapshot.value = nextSnapshot;
  }

  function reset() {
    snapshot.value = {
      ...emptySnapshot,
      updatedAt: new Date().toISOString()
    };
  }

  async function fetchSnapshot() {
    loading.value = true;
    try {
      const res = await fetch("/api/piles", { credentials: "include" });
      if (!res.ok) {
        await throwResponseError(res, "Load failed");
      }
      snapshot.value = (await res.json()) as DashboardSnapshot;
    } finally {
      loading.value = false;
    }
  }

  async function addPile(payload: { id: string; name: string; number: string; openNum: number; status: string; address: string }) {
    const res = await fetch("/api/piles", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      await throwResponseError(res, "add pile failed");
    }
    const pile = (await res.json()) as Pile;
    await fetchSnapshot();
    return pile;
  }

  async function deletePile(id: string) {
    const res = await fetch(`/api/piles/${id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) {
      await throwResponseError(res, "delete pile failed");
    }
    await fetchSnapshot();
  }

  async function updatePile(id: string, payload: { name: string; address: string; sortOrder: number }) {
    const res = await fetch(`/api/piles/${id}`, {
      method: "PATCH", credentials: "include", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!res.ok) await throwResponseError(res, "更新充电桩失败");
    await fetchSnapshot();
  }

  async function refreshFromCapture() {
    const res = await fetch("/api/refresh", {
      method: "POST",
      credentials: "include"
    });
    if (!res.ok) {
      await throwResponseError(res, "refresh failed");
    }
    snapshot.value = (await res.json()) as DashboardSnapshot;
  }

  async function updateCookie(cookie: string) {
    const res = await fetch("/api/session/cookie", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ cookie })
    });
    if (!res.ok) {
      await throwResponseError(res, "cookie update failed");
    }
    snapshot.value = (await res.json()) as DashboardSnapshot;
  }

  return {
    loading,
    snapshot,
    piles,
    stats,
    refresh,
    setSnapshot,
    reset,
    fetchSnapshot,
    addPile,
    deletePile,
    updatePile,
    refreshFromCapture,
    updateCookie
  };
});
