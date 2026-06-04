import { defineStore } from "pinia";
import { computed, ref } from "vue";
import type { CurrentUser } from "../types/dashboard";

export const useAuthStore = defineStore("auth", () => {
  const currentUser = ref<CurrentUser | null>(null);
  const loading = ref(false);

  const isLoggedIn = computed(() => currentUser.value !== null);
  const isAdmin = computed(() => currentUser.value?.role === "admin");

  async function fetchMe() {
    loading.value = true;
    try {
      const res = await fetch("/api/auth/me", { credentials: "include" });
      if (res.status === 401) {
        currentUser.value = null;
        return null;
      }
      if (!res.ok) {
        throw new Error(`Load user failed: ${res.status}`);
      }
      currentUser.value = (await res.json()) as CurrentUser;
      return currentUser.value;
    } finally {
      loading.value = false;
    }
  }

  async function login(username: string, password: string) {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password })
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "login failed");
    }
    currentUser.value = (await res.json()) as CurrentUser;
    return currentUser.value;
  }

  async function register(username: string, password: string) {
    const res = await fetch("/api/auth/register", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password })
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error ?? "register failed");
    }
    currentUser.value = (await res.json()) as CurrentUser;
    return currentUser.value;
  }

  async function logout() {
    await fetch("/api/auth/logout", {
      method: "POST",
      credentials: "include"
    });
    currentUser.value = null;
  }

  return {
    currentUser,
    loading,
    isLoggedIn,
    isAdmin,
    fetchMe,
    login,
    register,
    logout
  };
});
