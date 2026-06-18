<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

type TurnstileAPI = {
  render: (element: HTMLElement, options: Record<string, unknown>) => string;
  reset: (widgetId?: string) => void;
  remove: (widgetId: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}

const props = defineProps<{
  siteKey: string;
  action: "login" | "register";
}>();

const emit = defineEmits<{
  verified: [token: string];
  expired: [];
  error: [];
}>();

const container = ref<HTMLElement | null>(null);
let widgetID = "";

function loadScript() {
  return new Promise<void>((resolve, reject) => {
    if (window.turnstile) {
      resolve();
      return;
    }

    const existing = document.querySelector<HTMLScriptElement>("script[data-turnstile-script]");
    if (existing) {
      existing.addEventListener("load", () => resolve(), { once: true });
      existing.addEventListener("error", () => reject(new Error("Turnstile script failed")), { once: true });
      return;
    }

    const script = document.createElement("script");
    script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    script.async = true;
    script.defer = true;
    script.dataset.turnstileScript = "true";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("Turnstile script failed"));
    document.head.appendChild(script);
  });
}

async function renderWidget() {
  if (!props.siteKey || !container.value) return;
  await loadScript();
  await nextTick();
  if (!window.turnstile || !container.value) return;

  if (widgetID) {
    window.turnstile.remove(widgetID);
    widgetID = "";
  }
  widgetID = window.turnstile.render(container.value, {
    sitekey: props.siteKey,
    action: props.action,
    theme: "dark",
    size: "flexible",
    callback: (token: string) => emit("verified", token),
    "expired-callback": () => emit("expired"),
    "error-callback": () => emit("error"),
  });
}

function reset() {
  if (widgetID && window.turnstile) {
    window.turnstile.reset(widgetID);
  }
}

defineExpose({ reset });

onMounted(() => {
  void renderWidget();
});

watch(
  () => props.action,
  () => {
    void renderWidget();
  },
);

onBeforeUnmount(() => {
  if (widgetID && window.turnstile) {
    window.turnstile.remove(widgetID);
  }
});
</script>

<template>
  <div ref="container" class="turnstile-widget" />
</template>

<style scoped>
.turnstile-widget {
  min-height: 65px;
  width: 100%;
  display: grid;
  place-items: center;
}
</style>
