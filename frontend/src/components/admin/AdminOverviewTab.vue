<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";
import { ArrowRight, RefreshCw, UsersRound } from "@lucide/vue";
import AdminIssuesPanel from "./AdminIssuesPanel.vue";
import AdminMetricGrid from "./AdminMetricGrid.vue";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import { hasAdminRisk } from "@/utils/adminRisk";
import type { AdminOverview, AdminUserSummary, MetricPoint, SystemException } from "@/types/dashboard";

const AdminTrendChart = defineAsyncComponent(() => import("./AdminTrendChart.vue"));

const props = defineProps<{
  overview: AdminOverview;
  hourly: MetricPoint[];
  daily: MetricPoint[];
  issues: SystemException[];
  users: AdminUserSummary[];
  statsLoading: boolean;
  statsError: string;
  lastUpdatedAt?: string;
}>();

defineEmits<{
  "open-user": [userId: string];
  "open-users": [];
  "retry-stats": [];
}>();

const accountPreview = computed(() => [...props.users]
  .sort((left, right) => {
    const riskDifference = Number(hasAdminRisk(right)) - Number(hasAdminRisk(left));
    if (riskDifference) return riskDifference;
    return new Date(right.stats.lastRequestAt ?? 0).getTime() - new Date(left.stats.lastRequestAt ?? 0).getTime();
  })
  .slice(0, 5));

const credentialLabels = {
  unbound: "未绑定",
  waiting_device: "等待添加设备",
  healthy: "凭据正常",
  sync_failed: "同步失败",
  expired: "绑定已失效"
} as const;

function accountHealth(summary: AdminUserSummary) {
  if (summary.deviceIds.length === 0 && summary.credential.state === "unbound") return "未使用";
  return hasAdminRisk(summary) ? "需处理" : "正常";
}
</script>

<template>
  <div class="admin-overview space-y-4">
    <div v-if="statsError" class="admin-inline-error">
      <span>{{ statsError }}</span>
      <UiButton variant="outline" size="sm" @click="$emit('retry-stats')">
        <RefreshCw />重试
      </UiButton>
    </div>
    <template v-else>
      <AdminMetricGrid :overview="overview" />
      <div class="admin-overview-grid">
        <AdminTrendChart :hourly="hourly" :daily="daily" />
        <AdminIssuesPanel :issues="issues" @open-user="$emit('open-user', $event)" />
      </div>

      <article class="surface-panel overflow-hidden">
        <header class="admin-account-panel-head">
          <div class="admin-overview-panel-title">
            <UsersRound aria-hidden="true" />
            <span><h2>账户健康概览</h2><small>优先显示需要处理的账户</small></span>
          </div>
          <div class="admin-account-panel-actions">
            <span>{{ lastUpdatedAt ? `更新于 ${new Date(lastUpdatedAt).toLocaleString("zh-CN")}` : "尚未更新" }}</span>
            <button type="button" class="admin-panel-link" data-open-users @click="$emit('open-users')">
              全部用户<ArrowRight aria-hidden="true" />
            </button>
          </div>
        </header>
        <div v-if="accountPreview.length" class="divide-y divide-border">
          <div class="admin-account-head" aria-hidden="true">
            <span>用户</span><span>扫码凭据</span><span>设备</span><span>最近访问</span><span>状态</span><span />
          </div>
          <button
            v-for="summary in accountPreview"
            :key="summary.user.id"
            type="button"
            class="admin-account-row"
            @click="$emit('open-user', summary.user.id)"
          >
            <span class="admin-account-user">
              <strong class="block truncate">{{ summary.user.username }}</strong>
              <small class="admin-account-mobile-meta">{{ credentialLabels[summary.credential.state] }} · {{ summary.deviceIds.length }} / {{ summary.user.deviceLimit }} 台</small>
            </span>
            <span class="admin-account-credential">{{ credentialLabels[summary.credential.state] }}</span>
            <span>{{ summary.deviceIds.length }} / {{ summary.user.deviceLimit }} 台</span>
            <span>{{ summary.stats.lastRequestAt ? new Date(summary.stats.lastRequestAt).toLocaleString("zh-CN") : "暂无访问" }}</span>
            <UiBadge :variant="accountHealth(summary) === '需处理' ? 'destructive' : 'secondary'">
              {{ accountHealth(summary) }}
            </UiBadge>
            <ArrowRight class="size-4" aria-hidden="true" />
          </button>
        </div>
        <div v-else class="empty-compact">暂无账户数据。</div>
      </article>
    </template>
    <p v-if="statsLoading && users.length" class="sr-only" aria-live="polite">正在更新运营数据</p>
  </div>
</template>
