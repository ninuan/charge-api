<script setup lang="ts">
import { computed } from "vue";
import { NButton, NCard, NTag, NText } from "naive-ui";
import type { Pile, Port } from "../types/dashboard";

const props = defineProps<{
  pile: Pile;
}>();

const emit = defineEmits<{
  removePile: [pileId: string];
}>();

const inUseCount = computed(() => props.pile.ports.filter((p) => p.status === "in_use").length);

function statusLabel(port: Port) {
  if (port.status === "in_use") return "充电中";
  if (port.status === "offline") return "离线";
  return "空闲";
}

function statusClass(port: Port) {
  if (port.status === "in_use") return "port-cell in-use";
  if (port.status === "offline") return "port-cell offline";
  return "port-cell idle";
}
</script>

<template>
  <n-card class="pile-card" :bordered="false">
    <div class="pile-head">
      <div>
        <h3>{{ pile.name }}</h3>
        <n-text depth="3">桩号 {{ pile.number || pile.id }}</n-text>
      </div>
      <div class="head-actions">
        <n-tag :type="pile.online ? 'success' : 'error'" size="small">{{ pile.status }}</n-tag>
        <n-button quaternary type="error" @click="emit('removePile', pile.id)">删除桩</n-button>
      </div>
    </div>

    <div class="meta">
      <span>总口数 {{ pile.openNum }}</span>
      <span>使用中 {{ inUseCount }}</span>
      <span>来源 {{ pile.source === 'manual' ? '手动添加' : '远端接口' }}</span>
    </div>

    <div class="ports-grid">
      <div
        v-for="port in pile.ports"
        :key="port.id"
        :class="statusClass(port)"
      >
        <strong>{{ port.id }}</strong>
        <span>{{ statusLabel(port) }}</span>
        <small v-if="port.status === 'in_use' && port.usedText">已用 {{ port.usedText }}</small>
        <small v-if="port.status === 'in_use' && port.remainingText">剩余 {{ port.remainingText }}</small>
        <small v-if="port.status !== 'in_use'">--</small>
      </div>
    </div>
  </n-card>
</template>

<style scoped>
.pile-card {
  background: #1d2220;
  color: #f5f7fb;
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 8px;
}

.pile-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.pile-head h3 {
  margin: 0;
  font-size: 18px;
  letter-spacing: 0;
}

.head-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.meta {
  margin-top: 14px;
  display: flex;
  gap: 18px;
  font-size: 13px;
  color: #aeb8b1;
}

.ports-grid {
  margin-top: 16px;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(92px, 1fr));
  gap: 8px;
}

.port-cell {
  min-height: 86px;
  border-radius: 6px;
  padding: 10px 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: #fff;
  transition: filter 0.15s ease;
}

.port-cell strong {
  font-size: 15px;
}

.port-cell span {
  font-size: 12px;
}

.port-cell small {
  font-size: 11px;
  opacity: 0.9;
  line-height: 1.2;
  text-align: center;
}

.port-cell.idle {
  background: #33423d;
}

.port-cell.in-use {
  background: #0c7a54;
}

.port-cell.offline {
  background: #7f3d3d;
}
</style>
