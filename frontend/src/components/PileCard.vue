<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { BatteryCharging, ChevronDown, CircleCheck, Clock3, MapPin, Pencil, PlugZap, Trash2, WifiOff, Zap } from "@lucide/vue";
import { Input as UiInput } from "@/components/ui/input";
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
import { getPilePresentation } from "@/lib/pile-status";
import type { Pile, Port } from "@/types/dashboard";

const props = withDefaults(defineProps<{
  pile: Pile;
  visiblePortIds?: number[];
  filtering?: boolean;
}>(), {
  visiblePortIds: () => [],
  filtering: false
});
const emit = defineEmits<{
  removePile: [pileId: string];
  updatePile: [pileId: string, payload: { name: string; address: string; sortOrder: number }];
}>();
const confirmOpen = ref(false);
const editOpen = ref(false);
const collapsed = ref(false);
const portRegionID = computed(() => `pile-ports-${props.pile.id}`);
const edit = reactive({ name: props.pile.name, address: props.pile.address, sortOrder: props.pile.sortOrder ?? 0 });
watch(() => props.pile, (pile) => Object.assign(edit, { name: pile.name, address: pile.address, sortOrder: pile.sortOrder ?? 0 }));

const inUseCount = computed(() => props.pile.ports.filter((port) => port.status === "in_use").length);
const idleCount = computed(() => props.pile.ports.filter((port) => port.status === "idle").length);
const pilePresentation = computed(() => getPilePresentation(props.pile));
const displayedPorts = computed(() => {
  if (!props.filtering) return props.pile.ports;
  const visible = new Set(props.visiblePortIds);
  return props.pile.ports.filter((port) => visible.has(port.id));
});

function iconFor(port: Port) {
  const icon = getPortPresentation(port.status).icon;
  return icon === "Zap" ? Zap : icon === "WifiOff" ? WifiOff : CircleCheck;
}

function pileClass() {
  if (pilePresentation.value.tone === "charging") return "pile-panel pile-panel--charging";
  if (pilePresentation.value.tone === "offline") return "pile-panel pile-panel--offline";
  return "pile-panel pile-panel--idle";
}

function portClass(port: Port) {
  if (port.status === "in_use") return "port-tile port-tile--charging";
  if (port.status === "offline") return "port-tile port-tile--offline";
  return "port-tile port-tile--idle";
}

function pileIcon() {
  if (pilePresentation.value.tone === "charging") return BatteryCharging;
  if (pilePresentation.value.tone === "offline") return WifiOff;
  return CircleCheck;
}

function pileBadgeClass() {
  if (pilePresentation.value.tone === "charging") return "pile-state-badge pile-state-badge--charging";
  if (pilePresentation.value.tone === "offline") return "pile-state-badge pile-state-badge--offline";
  return "pile-state-badge pile-state-badge--idle";
}
</script>

<template>
  <article :class="pileClass()" :data-pile-state="pilePresentation.tone">
    <header class="flex flex-col gap-4 border-b border-border/70 p-5 sm:flex-row sm:items-start sm:justify-between lg:p-6">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="truncate text-xl font-bold tracking-tight">{{ pile.name }}</h2>
          <span :class="pileBadgeClass()">
            <component :is="pileIcon()" />
            {{ pilePresentation.label }}
          </span>
        </div>
        <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span class="inline-flex items-center gap-1.5"><PlugZap class="size-3.5" />桩号 {{ pile.number || pile.id }}</span>
          <span v-if="pile.address" class="inline-flex items-center gap-1.5"><MapPin class="size-3.5" />{{ pile.address }}</span>
        </div>
        <p v-if="filtering" class="mt-3 text-xs font-semibold text-primary">
          当前显示 {{ displayedPorts.length }} / {{ pile.ports.length }} 个匹配端口
        </p>
      </div>
      <div class="flex items-center justify-between gap-3 sm:justify-end">
        <div class="flex gap-4 text-right text-xs text-muted-foreground">
          <span><strong class="block font-mono text-lg text-foreground">{{ inUseCount }}</strong>使用中</span>
          <span><strong class="block font-mono text-lg text-foreground">{{ idleCount }}</strong>空闲</span>
        </div>
        <div class="flex">
          <UiButton
            variant="ghost"
            :aria-label="collapsed ? '展开充电口' : '收起充电口'"
            :aria-expanded="!collapsed"
            :aria-controls="portRegionID"
            @click="collapsed = !collapsed"
          >
            <ChevronDown :class="['transition-transform', { '-rotate-90': collapsed }]" />
          </UiButton>
          <UiButton variant="ghost" aria-label="编辑充电桩" title="编辑充电桩" @click="editOpen = true"><Pencil /></UiButton>
          <UiButton variant="ghost" aria-label="删除充电桩" title="删除充电桩" @click="confirmOpen = true"><Trash2 /></UiButton>
        </div>
      </div>
    </header>

    <div v-if="!collapsed" :id="portRegionID" class="grid grid-cols-2 gap-3 p-4 sm:grid-cols-3 lg:grid-cols-5 lg:p-6">
      <section
        v-for="port in displayedPorts"
        :key="port.id"
        :class="portClass(port)"
        :data-port-state="getPortPresentation(port.status).tone"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="font-mono text-lg font-bold tabular-nums">{{ String(port.id).padStart(2, "0") }}</span>
          <component :is="iconFor(port)" class="size-4" aria-hidden="true" />
        </div>
        <p class="mt-5 text-sm font-semibold">{{ getPortPresentation(port.status).label }}</p>
        <div class="mt-2 min-h-9 space-y-1 text-xs leading-4 opacity-80">
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

  <Dialog v-model:open="editOpen">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>编辑设备资料</DialogTitle>
        <DialogDescription>名称、地址和排序只影响你的个人看板。</DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="editOpen = false; emit('updatePile', pile.id, { ...edit })">
        <label class="form-field"><span>显示名称</span><UiInput v-model="edit.name" /></label>
        <label class="form-field"><span>安装地址</span><UiInput v-model="edit.address" /></label>
        <label class="form-field"><span>排序值</span><input v-model.number="edit.sortOrder" class="native-input" type="number"></label>
        <DialogFooter>
          <UiButton type="button" variant="ghost" @click="editOpen = false">取消</UiButton>
          <UiButton type="submit">保存修改</UiButton>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
