<script setup lang="ts">
import { computed } from "vue";
import { LogOut, ShieldCheck } from "@lucide/vue";
import { Button as UiButton } from "@/components/ui/button";
import { useAuthStore } from "@/stores/auth";
import { useRouter } from "vue-router";

defineProps<{
  eyebrow: string;
  title: string;
  description: string;
}>();

const auth = useAuthStore();
const router = useRouter();
const initial = computed(() => auth.currentUser?.username?.slice(0, 1).toUpperCase());

async function logout() {
  await auth.logout();
  await router.replace("/login");
}
</script>

<template>
  <div class="admin-console min-h-dvh bg-background text-foreground">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <header class="admin-console-topbar">
      <div class="admin-console-topbar-inner">
        <div class="flex min-w-0 items-center gap-3">
          <div class="brand-mark" aria-hidden="true"><span /></div>
          <div class="min-w-0">
            <p class="truncate text-sm font-bold tracking-tight">Charge Console</p>
            <p class="truncate text-xs text-muted-foreground">充电设施运营中心</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <div class="admin-console-identity">
            <span class="identity-avatar" aria-hidden="true">{{ initial }}</span>
            <span class="max-w-32 truncate text-sm font-semibold">{{ auth.currentUser?.username }}</span>
            <span class="identity-role"><ShieldCheck />管理员</span>
          </div>
          <UiButton variant="ghost" aria-label="退出登录" title="退出登录" @click="logout">
            <LogOut /><span class="hidden md:inline">退出</span>
          </UiButton>
        </div>
      </div>
    </header>

    <div class="admin-console-navigation">
      <div class="mx-auto max-w-[1480px] px-4 sm:px-6 lg:px-8"><slot name="navigation" /></div>
    </div>

    <main id="main-content" class="admin-console-main">
      <header class="admin-console-heading">
        <div class="min-w-0">
          <p class="section-kicker">{{ eyebrow }}</p>
          <h1>{{ title }}</h1>
          <p>{{ description }}</p>
        </div>
        <div class="admin-console-heading-side">
          <slot name="status" />
          <div class="admin-console-heading-actions"><slot name="actions" /></div>
        </div>
      </header>
      <slot />
    </main>
  </div>
</template>
