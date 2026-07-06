import { defineStore } from "pinia";
import { computed, ref } from "vue";

export interface YYBBindingStatus {
  bound: boolean;
  openidSuffix?: string;
  nickname?: string;
  status?: string;
  boundAt?: string;
  lastCheckedAt?: string;
  cookieSynced?: boolean;
  message?: string;
}

export interface YYBQRSession {
  sessionId: string;
  imageUrl?: string;
  imageBase64?: string;
  status?: string;
}

export interface YYBQRPollResult {
  sessionId: string;
  status: string;
  message?: string;
}

async function throwResponseError(res: Response, fallback: string): Promise<never> {
  throw new Error(await responseErrorMessage(res, fallback));
}

async function responseErrorMessage(res: Response, fallback: string): Promise<string> {
  if (res.status === 401) {
    return "登录已失效，请重新登录";
  }
  const err = await res.json().catch(() => ({ error: fallback }));
  return err.error ?? fallback;
}

export const useYYBStore = defineStore("yyb", () => {
  const binding = ref<YYBBindingStatus>({ bound: false });
  const qr = ref<YYBQRSession | null>(null);
  const poll = ref<YYBQRPollResult | null>(null);
  const loading = ref(false);
  const polling = ref(false);
  const confirming = ref(false);
  let activePoll: Promise<YYBQRPollResult> | null = null;

  const isBound = computed(() => binding.value.bound);

  async function fetchBinding() {
    const res = await fetch("/api/session/yyb-binding", { credentials: "include" });
    if (!res.ok) {
      await throwResponseError(res, "读取扫码绑定状态失败");
    }
    binding.value = (await res.json()) as YYBBindingStatus;
    return binding.value;
  }

  async function createQR() {
    loading.value = true;
    try {
      const res = await fetch("/api/session/yyb-qr", {
        method: "POST",
        credentials: "include"
      });
      if (!res.ok) {
        await throwResponseError(res, "生成扫码登录二维码失败");
      }
      qr.value = (await res.json()) as YYBQRSession;
      poll.value = null;
      return qr.value;
    } finally {
      loading.value = false;
    }
  }

  async function pollQR() {
    if (!qr.value?.sessionId) {
      throw new Error("请先生成二维码");
    }
    const sessionId = qr.value.sessionId;
    if (activePoll) {
      return activePoll;
    }
    polling.value = true;
    activePoll = (async () => {
      const res = await fetch(`/api/session/yyb-qr/${encodeURIComponent(sessionId)}/poll`, {
        credentials: "include"
      });
      if (!res.ok) {
        const errorMessage = await responseErrorMessage(res, "读取扫码状态失败");
        if (errorMessage.includes("qr session not found")) {
          qr.value = null;
          poll.value = null;
          throw new Error("二维码会话已失效，请重新生成二维码");
        }
        throw new Error(errorMessage);
      }
      poll.value = (await res.json()) as YYBQRPollResult;
      return poll.value;
    })();
    try {
      return await activePoll;
    } finally {
      polling.value = false;
      activePoll = null;
    }
  }

  async function confirmQR() {
    if (!qr.value?.sessionId) {
      throw new Error("请先生成二维码");
    }
    confirming.value = true;
    try {
      const res = await fetch(`/api/session/yyb-qr/${encodeURIComponent(qr.value.sessionId)}/confirm`, {
        method: "POST",
        credentials: "include"
      });
      if (!res.ok) {
        await throwResponseError(res, "确认扫码登录失败");
      }
      binding.value = (await res.json()) as YYBBindingStatus;
      qr.value = null;
      poll.value = null;
      return binding.value;
    } finally {
      confirming.value = false;
    }
  }

  async function clearBinding() {
    const res = await fetch("/api/session/yyb-binding", {
      method: "DELETE",
      credentials: "include"
    });
    if (!res.ok && res.status !== 204) {
      await throwResponseError(res, "解绑扫码登录失败");
    }
    binding.value = { bound: false };
    qr.value = null;
    poll.value = null;
  }

  return {
    binding,
    qr,
    poll,
    loading,
    polling,
    confirming,
    isBound,
    fetchBinding,
    createQR,
    pollQR,
    confirmQR,
    clearBinding
  };
});
