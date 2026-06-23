<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { createDiscreteApi } from "naive-ui";
import { Activity, BatteryCharging, CircleDot, Clock3, PlugZap, TriangleAlert } from "@lucide/vue";
import AppShell from "@/components/AppShell.vue";
import AddPileDialog from "@/components/AddPileDialog.vue";
import ConnectionBadge from "@/components/ConnectionBadge.vue";
import CookieDialog from "@/components/CookieDialog.vue";
import MetricCard from "@/components/MetricCard.vue";
import PileCard from "@/components/PileCard.vue";
import { Button as UiButton } from "@/components/ui/button";
import { useDashboardStream } from "@/composables/useDashboardStream";
import { useDashboardStore } from "@/stores/dashboard";

const store = useDashboardStore();
const { message } = createDiscreteApi(["message"]);
const refreshing = ref(false);
const stream = useDashboardStream((snapshot) => store.setSnapshot(snapshot));
const refreshTone = computed(() => store.refresh.partial || store.refresh.cached ? "warning" : "success");

function formatTime(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

async function refresh() {
  refreshing.value = true;
  try {
    await store.refreshFromCapture();
    message.success(store.refresh.message || "设备状态已刷新");
  } catch (error) {
    message.error((error as Error).message);
  } finally {
    refreshing.value = false;
  }
}

async function removePile(id: string) {
  try {
    await store.deletePile(id);
    message.success("充电桩已移除");
  } catch (error) {
    message.error((error as Error).message);
  }
}

onMounted(async () => {
  try {
    await store.fetchSnapshot();
    stream.connect();
  } catch (error) {
    message.error((error as Error).message);
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
        <CookieDialog />
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

    <section class="mt-7 space-y-4" aria-label="充电桩列表">
      <div v-if="store.loading" class="grid gap-4">
        <div v-for="item in 2" :key="item" class="skeleton-panel" />
      </div>
      <PileCard
        v-for="pile in store.piles"
        v-else
        :key="pile.id"
        :pile="pile"
        @remove-pile="removePile"
      />
      <div v-if="!store.loading && store.piles.length === 0" class="empty-state">
        <span><PlugZap /></span>
        <h2>还没有充电桩</h2>
        <p>先配置你的 Cookie，然后添加设备长 ID。设备状态不会在后台自动请求。</p>
        <div class="mt-5 flex flex-wrap justify-center gap-2"><CookieDialog /><AddPileDialog /></div>
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
