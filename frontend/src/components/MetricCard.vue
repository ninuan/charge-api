<script setup lang="ts">
import type { Component } from "vue";

withDefaults(defineProps<{
  label: string;
  value: string | number;
  detail?: string;
  tone?: "neutral" | "green" | "amber" | "red" | "blue";
  icon: Component;
}>(), {
  detail: "",
  tone: "neutral"
});
</script>

<template>
  <article :class="['metric-panel', `metric-panel--${tone}`]">
    <div class="flex items-start justify-between gap-3">
      <div>
        <p class="metric-label text-sm font-medium text-muted-foreground">{{ label }}</p>
        <p class="metric-value mt-3 font-mono text-3xl font-bold tracking-[-0.05em] tabular-nums lg:text-4xl">{{ value }}</p>
      </div>
      <span class="metric-icon" aria-hidden="true">
        <component :is="icon" />
      </span>
    </div>
    <p v-if="detail" class="metric-detail mt-4 text-xs leading-5 text-muted-foreground">{{ detail }}</p>
  </article>
</template>
