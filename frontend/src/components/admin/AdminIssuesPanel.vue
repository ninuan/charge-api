<script setup lang="ts">
import { AlertTriangle, ArrowRight } from "@lucide/vue";
import type { SystemException } from "@/types/dashboard";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";

defineProps<{ issues: SystemException[] }>();
defineEmits<{ "open-user": [userId: string] }>();

function displayMessage(message: string) {
  return message.replace(/Cookie/gi, "扫码凭据");
}
</script>

<template>
  <article class="admin-issues-panel surface-panel overflow-hidden">
    <header class="admin-overview-panel-head">
      <div class="admin-overview-panel-title admin-overview-panel-title--danger">
        <AlertTriangle aria-hidden="true" />
        <span><h2>待处理事项</h2><small>按严重程度和时间排序</small></span>
      </div>
    </header>
    <div v-if="issues.length" class="divide-y divide-border">
      <div v-for="item in issues.slice(0, 5)" :key="item.id" class="admin-issue-row">
        <i class="admin-issue-severity" :data-level="item.level" aria-hidden="true" />
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <strong class="truncate">{{ item.username }}</strong>
            <UiBadge :variant="item.level === 'critical' ? 'destructive' : 'outline'">
              {{ item.level === "critical" ? "紧急" : "注意" }}
            </UiBadge>
          </div>
          <p class="mt-1 text-sm text-muted-foreground">{{ displayMessage(item.message) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">{{ new Date(item.time).toLocaleString("zh-CN") }}</p>
        </div>
        <UiButton
          variant="outline"
          size="sm"
          :data-issue-user="item.userId"
          @click="$emit('open-user', item.userId)"
        >
          查看<ArrowRight />
        </UiButton>
      </div>
    </div>
    <div v-else class="empty-compact">当前没有需要处理的异常。</div>
  </article>
</template>
