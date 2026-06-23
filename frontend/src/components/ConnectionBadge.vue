<script setup lang="ts">
import { CircleAlert, Radio, WifiOff } from "@lucide/vue";

defineProps<{
  state: "connected" | "connecting" | "offline";
}>();
</script>

<template>
  <span :class="['connection-badge', `connection-badge--${state}`]" role="status">
    <Radio v-if="state === 'connected'" />
    <CircleAlert v-else-if="state === 'connecting'" />
    <WifiOff v-else />
    {{ state === "connected" ? "实时连接" : state === "connecting" ? "正在连接" : "连接已断开" }}
  </span>
</template>
