<script setup lang="ts">
import { computed } from "vue";
import type { AdminHealth, HealthState } from "@/types/dashboard";
import { Button as UiButton } from "@/components/ui/button";

const props = defineProps<{
  health: AdminHealth | null;
  loading: boolean;
  error: string;
}>();

defineEmits<{ retry: [] }>();

const services = computed(() => props.health ? [
  { key: "charge", label: "Charge", ...props.health.charge },
  { key: "database", label: "SQLite", ...props.health.database },
  { key: "yyb", label: "扫码服务", ...props.health.yyb }
] : []);

function stateLabel(state: HealthState) {
  if (state === "healthy") return "正常";
  if (state === "degraded") return "异常";
  return "不可用";
}
</script>

<template>
  <section class="admin-health-strip" aria-label="系统健康状态">
    <div v-if="loading && !health" class="admin-health-loading">正在检查服务状态…</div>
    <div v-else-if="error && !health" class="admin-inline-error">
      <span>{{ error }}</span>
      <UiButton size="sm" variant="outline" @click="$emit('retry')">重新检查</UiButton>
    </div>
    <template v-else>
      <span
        v-for="service in services"
        :key="service.key"
        class="admin-health-chip"
        :data-state="service.state"
        :title="service.message"
        :aria-label="`${service.label}：${service.message}`"
      >
        <i aria-hidden="true" />
        <strong>{{ service.label }}</strong>
        <small>{{ stateLabel(service.state) }}</small>
      </span>
      <p v-if="health" class="admin-health-time">
        {{ new Date(health.checkedAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }) }} 更新
      </p>
    </template>
  </section>
</template>
