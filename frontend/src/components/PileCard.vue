<script setup lang="ts">
import { computed } from "vue";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import {
  Card as UiCard,
  CardContent as UiCardContent,
  CardHeader as UiCardHeader,
  CardTitle as UiCardTitle,
} from "@/components/ui/card";
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
  <UiCard class="pile-card">
    <UiCardHeader class="pile-head">
      <div>
        <UiCardTitle>{{ pile.name }}</UiCardTitle>
        <p>桩号 {{ pile.number || pile.id }}</p>
      </div>
      <div class="head-actions">
        <UiBadge :variant="pile.online ? 'default' : 'destructive'">{{ pile.status }}</UiBadge>
        <UiButton variant="destructive" size="sm" @click="emit('removePile', pile.id)">删除桩</UiButton>
      </div>
    </UiCardHeader>

    <UiCardContent>
      <div class="meta-grid">
        <div>
          <span>总口数</span>
          <strong>{{ pile.openNum }}</strong>
        </div>
        <div>
          <span>使用中</span>
          <strong>{{ inUseCount }}</strong>
        </div>
        <div>
          <span>来源</span>
          <strong>{{ pile.source === 'manual' ? '手动添加' : '远端接口' }}</strong>
        </div>
      </div>

      <div class="ports-grid">
        <div v-for="port in pile.ports" :key="port.id" :class="statusClass(port)">
          <div class="port-top">
            <strong>{{ port.id }}</strong>
            <span>{{ statusLabel(port) }}</span>
          </div>
          <div class="port-time">
            <small v-if="port.status === 'in_use' && port.usedText">已用 {{ port.usedText }}</small>
            <small v-if="port.status === 'in_use' && port.remainingText">剩余 {{ port.remainingText }}</small>
            <small v-if="port.status !== 'in_use'">--</small>
          </div>
        </div>
      </div>
    </UiCardContent>
  </UiCard>
</template>

<style scoped>
.pile-card {
  border-color: rgb(255 255 255 / 10%);
  background:
    radial-gradient(circle at top left, rgb(58 171 116 / 13%), transparent 38%),
    rgb(17 20 20 / 94%);
}

.pile-head {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  gap: 14px;
  align-items: flex-start;
}

.pile-head p {
  margin: 8px 0 0;
  color: #9fa7a1;
  font-size: 13px;
}

.head-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}

.meta-grid div {
  padding: 10px 12px;
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 12px;
  background: rgb(255 255 255 / 4%);
}

.meta-grid span {
  display: block;
  color: #9fa7a1;
  font-size: 12px;
}

.meta-grid strong {
  display: block;
  margin-top: 4px;
  font-size: 18px;
}

.ports-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(104px, 1fr));
  gap: 10px;
}

.port-cell {
  position: relative;
  overflow: hidden;
  min-height: 94px;
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 14px;
  padding: 11px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  color: #fff;
}

.port-cell::after {
  content: "";
  position: absolute;
  inset: auto -20px -30px auto;
  width: 70px;
  height: 70px;
  border-radius: 999px;
  opacity: 0.22;
}

.port-top,
.port-time {
  position: relative;
  z-index: 1;
}

.port-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.port-top strong {
  font-size: 18px;
}

.port-top span {
  font-size: 12px;
  opacity: 0.88;
}

.port-time {
  display: grid;
  gap: 3px;
  margin-top: 10px;
  color: rgb(255 255 255 / 86%);
  font-size: 11px;
  line-height: 1.25;
}

.port-cell.idle {
  background: linear-gradient(145deg, rgb(51 66 61 / 92%), rgb(34 42 39 / 92%));
}

.port-cell.idle::after {
  background: #a7c4b4;
}

.port-cell.in-use {
  background: linear-gradient(145deg, rgb(12 122 84 / 96%), rgb(12 75 62 / 96%));
}

.port-cell.in-use::after {
  background: #7df0b0;
}

.port-cell.offline {
  background: linear-gradient(145deg, rgb(127 61 61 / 96%), rgb(72 45 45 / 96%));
}

.port-cell.offline::after {
  background: #ff9d91;
}

@media (max-width: 760px) {
  .pile-head {
    flex-direction: column;
  }

  .head-actions {
    justify-content: flex-start;
  }

  .meta-grid {
    grid-template-columns: 1fr;
  }
}
</style>
