<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { LogOut, RefreshCw, ShieldCheck, UserRound } from "@lucide/vue";
import { Button as UiButton } from "@/components/ui/button";
import { Badge as UiBadge } from "@/components/ui/badge";
import { useAuthStore } from "@/stores/auth";
import SecurityDialog from "@/components/SecurityDialog.vue";

const props = withDefaults(defineProps<{
  title: string;
  description: string;
  refreshing?: boolean;
  showRefresh?: boolean;
}>(), {
  refreshing: false,
  showRefresh: false
});

const emit = defineEmits<{
  refresh: [];
}>();

const auth = useAuthStore();
const router = useRouter();
const roleLabel = computed(() => auth.isAdmin ? "管理员" : "普通用户");
const roleIcon = computed(() => auth.isAdmin ? ShieldCheck : UserRound);

async function logout() {
  await auth.logout();
  await router.replace("/login");
}
</script>

<template>
  <div class="min-h-dvh bg-background text-foreground">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <div class="ambient-grid" aria-hidden="true" />
    <header class="sticky top-0 z-40 border-b border-border/70 bg-background/88 backdrop-blur-xl">
      <div class="mx-auto flex min-h-16 max-w-[1480px] items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
        <div class="flex min-w-0 items-center gap-3">
          <div class="brand-mark" aria-hidden="true">
            <span />
          </div>
          <div class="min-w-0">
            <p class="truncate text-sm font-bold tracking-tight">Charge Console</p>
            <p class="truncate text-xs text-muted-foreground">充电设施运营中心</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <UiButton
            v-if="props.showRefresh"
            variant="outline"
            :disabled="props.refreshing"
            @click="emit('refresh')"
          >
            <RefreshCw :class="{ 'animate-spin': props.refreshing }" />
            <span class="hidden sm:inline">{{ props.refreshing ? "刷新中" : "刷新状态" }}</span>
          </UiButton>
          <SecurityDialog v-if="!auth.isAdmin" />
          <div class="identity-pill hidden sm:flex">
            <span class="identity-avatar" aria-hidden="true">{{ auth.currentUser?.username?.slice(0, 1).toUpperCase() }}</span>
            <span class="max-w-32 truncate text-sm font-semibold">{{ auth.currentUser?.username }}</span>
            <UiBadge :variant="auth.isAdmin ? 'default' : 'secondary'">
              <component :is="roleIcon" />
              {{ roleLabel }}
            </UiBadge>
          </div>
          <UiButton variant="ghost" aria-label="退出登录" title="退出登录" @click="logout">
            <LogOut />
            <span class="hidden md:inline">退出</span>
          </UiButton>
        </div>
      </div>
    </header>

    <main id="main-content" class="relative mx-auto max-w-[1480px] px-4 py-7 sm:px-6 lg:px-8 lg:py-10">
      <section class="mb-7 flex flex-col justify-between gap-4 md:flex-row md:items-end">
        <div>
          <p class="section-kicker">{{ auth.isAdmin ? "Operations intelligence" : "Live infrastructure" }}</p>
          <h1 class="mt-2 text-3xl font-bold tracking-[-0.04em] sm:text-4xl lg:text-5xl">{{ title }}</h1>
          <p class="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">{{ description }}</p>
        </div>
        <slot name="heading-actions" />
      </section>
      <slot />
    </main>
  </div>
</template>
