<script setup lang="ts">
import { Activity, AlertTriangle, Cable, RadioTower, UsersRound } from "@lucide/vue";
import MetricCard from "@/components/MetricCard.vue";
import type { AdminOverview } from "@/types/dashboard";

defineProps<{ overview: AdminOverview }>();
</script>

<template>
  <section class="admin-metric-grid" aria-label="运营核心指标">
    <MetricCard label="待处理异常" :value="overview.openIssues" detail="当前仍未恢复的问题" tone="red" :icon="AlertTriangle" />
    <MetricCard label="远端成功率" :value="`${overview.remoteSuccessRate}%`" detail="最近 24 小时设备请求" tone="green" :icon="RadioTower" />
    <MetricCard label="活跃用户" :value="overview.activeUsers" detail="最近 24 小时" tone="blue" :icon="UsersRound" />
    <MetricCard label="受管设备" :value="overview.managedDevices" detail="当前已添加设备" :icon="Cable" />
    <MetricCard label="离线充电口" :value="overview.offlinePorts" detail="当前设备快照" tone="amber" :icon="Activity" />
  </section>
</template>
