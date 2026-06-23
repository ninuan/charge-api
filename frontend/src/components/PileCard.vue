<script setup lang="ts">
import { computed, ref } from "vue";
import { CircleCheck, Clock3, MapPin, MoreVertical, PlugZap, Trash2, WifiOff, Zap } from "@lucide/vue";
import { Badge as UiBadge } from "@/components/ui/badge";
import { Button as UiButton } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { getPortPresentation } from "@/lib/port-status";
import type { Pile, Port } from "@/types/dashboard";

const props = defineProps<{ pile: Pile }>();
const emit = defineEmits<{ removePile: [pileId: string] }>();
const confirmOpen = ref(false);

const inUseCount = computed(() => props.pile.ports.filter((port) => port.status === "in_use").length);
const idleCount = computed(() => props.pile.ports.filter((port) => port.status === "idle").length);

function iconFor(port: Port) {
  const icon = getPortPresentation(port.status).icon;
  return icon === "Zap" ? Zap : icon === "WifiOff" ? WifiOff : CircleCheck;
}
</script>

<template>
  <article class="pile-panel">
    <header class="flex flex-col gap-4 border-b border-border/70 p-5 sm:flex-row sm:items-start sm:justify-between lg:p-6">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="truncate text-xl font-bold tracking-tight">{{ pile.name }}</h2>
          <UiBadge :variant="pile.online ? 'default' : 'destructive'">
            {{ pile.online ? "在线" : "离线" }}
          </UiBadge>
        </div>
        <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span class="inline-flex items-center gap-1.5"><PlugZap class="size-3.5" />桩号 {{ pile.number || pile.id }}</span>
          <span v-if="pile.address" class="inline-flex items-center gap-1.5"><MapPin class="size-3.5" />{{ pile.address }}</span>
        </div>
      </div>
      <div class="flex items-center justify-between gap-3 sm:justify-end">
        <div class="flex gap-4 text-right text-xs text-muted-foreground">
          <span><strong class="block font-mono text-lg text-foreground">{{ inUseCount }}</strong>使用中</span>
          <span><strong class="block font-mono text-lg text-foreground">{{ idleCount }}</strong>空闲</span>
        </div>
        <UiButton variant="ghost" aria-label="删除充电桩" title="删除充电桩" @click="confirmOpen = true">
          <MoreVertical />
        </UiButton>
      </div>
    </header>

    <div class="grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 lg:grid-cols-5 lg:p-6">
      <section
        v-for="port in pile.ports"
        :key="port.id"
        :class="['port-tile', `port-tile--${getPortPresentation(port.status).tone}`]"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="font-mono text-lg font-bold tabular-nums">{{ String(port.id).padStart(2, "0") }}</span>
          <component :is="iconFor(port)" class="size-4" aria-hidden="true" />
        </div>
        <p class="mt-5 text-sm font-semibold">{{ getPortPresentation(port.status).label }}</p>
        <div class="mt-2 min-h-9 space-y-1 text-[11px] leading-4 opacity-80">
          <p v-if="port.status === 'in_use' && port.usedText" class="flex items-center gap-1">
            <Clock3 class="size-3" />已用 {{ port.usedText }}
          </p>
          <p v-if="port.status === 'in_use' && port.remainingText">剩余 {{ port.remainingText }}</p>
          <p v-if="port.status !== 'in_use'">等待连接</p>
        </div>
      </section>
    </div>
  </article>

  <Dialog v-model:open="confirmOpen">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>删除“{{ pile.name }}”？</DialogTitle>
        <DialogDescription>设备会从你的看板中移除。该操作不会影响充电桩本身。</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <UiButton variant="ghost" @click="confirmOpen = false">取消</UiButton>
        <UiButton
          variant="destructive"
          @click="confirmOpen = false; emit('removePile', pile.id)"
        >
          <Trash2 />确认删除
        </UiButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
