<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { createDiscreteApi } from "naive-ui";
import { Activity, BatteryCharging, CircleDot, Clock3, PlugZap, RotateCcw, Search, TriangleAlert } from "@lucide/vue";
import { Input as UiInput } from "@/components/ui/input";
import AppShell from "@/components/AppShell.vue";
import AddPileDialog from "@/components/AddPileDialog.vue";
import ConnectionBadge from "@/components/ConnectionBadge.vue";
import MetricCard from "@/components/MetricCard.vue";
import PileCard from "@/components/PileCard.vue";
import UsageGuideDialog from "@/components/UsageGuideDialog.vue";
import YybLoginDialog from "@/components/YybLoginDialog.vue";
import { Button as UiButton } from "@/components/ui/button";
import { useDashboardStream } from "@/composables/useDashboardStream";
import { useAuthStore } from "@/stores/auth";
import { useDashboardStore } from "@/stores/dashboard";
import type { PortStatus } from "@/types/dashboard";

const store = useDashboardStore();
const auth = useAuthStore();
const router = useRouter();
const { message } = createDiscreteApi(["message"]);
const refreshing = ref(false);
const search = ref("");
const filter = ref<"all" | "idle" | "charging" | "offline">("all");
const stream = useDashboardStream((snapshot) => store.setSnapshot(snapshot));
const refreshTone = computed(() => store.refresh.partial || store.refresh.cached ? "warning" : "success");
const statusMap: Record<"all" | "idle" | "charging" | "offline", PortStatus | null> = {
  all: null,
  idle: "idle",
  charging: "in_use",
  offline: "offline"
};
const visiblePiles = computed(() => store.piles.flatMap((pile) => {
  const query = search.value.trim().toLowerCase();
  const isPortNumberQuery = /^\d{1,2}$/.test(query);
  const pileMatches = !query || (!isPortNumberQuery &&
    `${pile.name} ${pile.number} ${pile.address} ${pile.id}`.toLowerCase().includes(query));
  const requiredStatus = statusMap[filter.value];
  const ports = pile.ports.filter((port) => {
    const portMatches = pileMatches || (isPortNumberQuery && port.id === Number(query));
    return portMatches && (!requiredStatus || port.status === requiredStatus);
  });
  return ports.length ? [{ pile, portIds: ports.map((port) => port.id) }] : [];
}));
const visiblePortCount = computed(() => visiblePiles.value.reduce((total, entry) => total + entry.portIds.length, 0));
const hasActiveFilter = computed(() => Boolean(search.value.trim()) || filter.value !== "all");

async function handlePageError(error: unknown) {
  if ((error as Error).message.includes("登录已失效")) {
    auth.clearSession();
    await router.replace("/login");
    return;
  }
  message.error((error as Error).message);
}

function formatTime(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function clearFilters() {
  search.value = "";
  filter.value = "all";
}

async function refresh() {
  refreshing.value = true;
  try {
    await store.refreshFromCapture();
    message.success(store.refresh.message || "设备状态已刷新");
  } catch (error) {
    await handlePageError(error);
  } finally {
    refreshing.value = false;
  }
}

async function removePile(id: string) {
  try {
    await store.deletePile(id);
    message.success("充电桩已移除");
  } catch (error) {
    await handlePageError(error);
  }
}

async function updatePile(id: string, payload: { name: string; address: string; sortOrder: number }) {
  try {
    await store.updatePile(id, payload);
    message.success("设备资料已更新");
  } catch (error) {
    await handlePageError(error);
  }
}

onMounted(async () => {
  try {
    await store.fetchSnapshot();
    stream.connect();
  } catch (error) {
    await handlePageError(error);
  }
});
</script>

<template>
  <AppShell
    title="充电桩运营看板"
    description="查看每个端口的占用、已用时间和剩余时间。只有点击刷新按钮时，系统才会请求远端充电桩接口。"
    show-refresh
    :refreshing="refreshing"
    @refresh="refresh"
  >
    <template #heading-actions>
      <div class="flex flex-wrap items-center gap-2">
        <ConnectionBadge :state="stream.state.value" />
        <UsageGuideDialog />
        <YybLoginDialog />
        <AddPileDialog />
      </div>
    </template>

    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="设备指标">
      <MetricCard label="充电桩" :value="store.stats.pileCount" detail="当前账户添加的设备" :icon="PlugZap" />
      <MetricCard label="全部端口" :value="store.stats.portCount" detail="所有设备端口总和" tone="blue" :icon="CircleDot" />
      <MetricCard label="正在使用" :value="store.stats.inUsePortCount" detail="当前正在充电的端口" tone="amber" :icon="BatteryCharging" />
      <MetricCard label="异常端口" :value="store.stats.offlinePorts" detail="离线或暂时不可访问" tone="red" :icon="TriangleAlert" />
    </section>

    <section v-if="store.refresh.message" :class="['refresh-status', `refresh-status--${refreshTone}`]" role="status">
      <Activity class="size-5 shrink-0" />
      <div class="min-w-0">
        <p class="font-semibold">{{ store.refresh.message }}</p>
        <p class="mt-1 text-xs opacity-75">
          上次远端请求 {{ formatTime(store.refresh.lastRemoteAt) }} · 下次允许请求 {{ formatTime(store.refresh.nextRemoteAt) }}
          <span v-if="store.refresh.nextRetryAt"> · 退避至 {{ formatTime(store.refresh.nextRetryAt) }}</span>
        </p>
      </div>
    </section>

    <section class="filter-panel mt-7" aria-labelledby="port-filter-title">
      <div class="min-w-0">
        <h2 id="port-filter-title" class="text-sm font-bold">查找充电口</h2>
        <p class="mt-1 text-xs leading-5 text-muted-foreground">按充电桩信息、端口号或端口状态筛选，结果精确到单个充电口。</p>
      </div>
      <div class="filter-controls">
        <label class="relative min-w-0 flex-1">
          <span class="sr-only">搜索充电桩或端口号</span>
          <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <UiInput v-model="search" class="pl-10" placeholder="名称、桩号、地址或端口号" />
        </label>
        <label>
          <span class="sr-only">按充电口状态筛选</span>
          <select v-model="filter" class="native-input sm:w-40">
            <option value="all">全部充电口</option>
            <option value="idle">仅看空闲</option>
            <option value="charging">仅看充电中</option>
            <option value="offline">仅看离线</option>
          </select>
        </label>
      </div>
      <p class="filter-summary" aria-live="polite">
        {{ visiblePiles.length }} 台桩 · {{ visiblePortCount }} 个充电口
      </p>
    </section>

    <section class="mt-4 space-y-4" aria-label="充电桩列表">
      <div v-if="store.loading" class="grid gap-4">
        <div v-for="item in 2" :key="item" class="skeleton-panel" />
      </div>
      <PileCard
        v-for="entry in visiblePiles"
        v-else
        :key="entry.pile.id"
        :pile="entry.pile"
        :visible-port-ids="entry.portIds"
        :filtering="hasActiveFilter"
        @remove-pile="removePile"
        @update-pile="updatePile"
      />
      <div v-if="!store.loading && visiblePiles.length === 0" class="empty-state">
        <span><PlugZap /></span>
        <h2>{{ store.piles.length ? "没有匹配的充电口" : "还没有充电桩" }}</h2>
        <p>{{ store.piles.length ? "调整搜索内容或状态条件，查看其他充电口。" : "建议先完成扫码登录，再通过添加入口录入桩号；系统会在添加设备时自动维护登录凭据。" }}</p>
        <UiButton v-if="store.piles.length && hasActiveFilter" class="mt-5" variant="outline" @click="clearFilters">
          <RotateCcw />清除筛选条件
        </UiButton>
        <div v-if="!store.piles.length" class="mt-5 flex flex-wrap justify-center gap-2"><YybLoginDialog /><AddPileDialog /></div>
      </div>
    </section>

    <div class="mobile-refresh-bar">
      <UiButton class="w-full" :disabled="refreshing" @click="refresh">
        <Clock3 :class="{ 'animate-spin': refreshing }" />
        {{ refreshing ? "刷新中…" : "主动刷新设备状态" }}
      </UiButton>
    </div>
  </AppShell>
</template>
