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

export const useDashboardStore = defineStore("dashboard", () => {
  const snapshot = ref<DashboardSnapshot>(emptySnapshot);
  const loading = ref(false);

  const piles = computed(() => snapshot.value.piles);
  const stats = computed(() => snapshot.value.statistics);
  const refresh = computed(() => snapshot.value.refresh);

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
        throw new Error(`Load failed: ${res.status}`);
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
      const err = await res.json();
      throw new Error(err.error ?? "add pile failed");
    }
    return (await res.json()) as Pile;
  }

  async function deletePile(id: string) {
    const res = await fetch(`/api/piles/${id}`, {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) {
      const err = await res.json();
      throw new Error(err.error ?? "delete pile failed");
    }
  }

  async function refreshFromCapture() {
    const res = await fetch("/api/refresh", {
      method: "POST",
      credentials: "include"
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "refresh failed");
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
      const err = await res.json();
      throw new Error(err.error ?? "cookie update failed");
    }
    snapshot.value = (await res.json()) as DashboardSnapshot;
  }

  return {
    loading,
    snapshot,
    piles,
    stats,
    refresh,
    reset,
    fetchSnapshot,
    addPile,
    deletePile,
    refreshFromCapture,
    updateCookie
  };
});
