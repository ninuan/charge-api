<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { PieChart } from "echarts/charts";
import { LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { hasTrafficData } from "@/lib/traffic";

use([PieChart, LegendComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{
  remote: number;
  cached: number;
  failed: number;
}>();

const chartElement = ref<HTMLDivElement | null>(null);
let chart: EChartsType | null = null;

function render() {
  if (!chartElement.value) return;
  if (!hasTrafficData(props.remote, props.cached, props.failed)) {
    chart?.dispose();
    chart = null;
    return;
  }
  chart ??= init(chartElement.value);
  chart.setOption({
    animationDuration: 350,
    tooltip: { trigger: "item" },
    legend: {
      bottom: 0,
      textStyle: { color: "#68736d" }
    },
    series: [{
      type: "pie",
      radius: ["55%", "78%"],
      center: ["50%", "44%"],
      label: { show: false },
      itemStyle: { borderRadius: 7, borderWidth: 3, borderColor: "transparent" },
      data: [
        { name: "远端请求", value: props.remote, itemStyle: { color: "#16784d" } },
        { name: "缓存命中", value: props.cached, itemStyle: { color: "#d8992e" } },
        { name: "失败请求", value: props.failed, itemStyle: { color: "#d15a4b" } }
      ]
    }]
  });
}

function resize() {
  chart?.resize();
}

watch(() => [props.remote, props.cached, props.failed], async () => {
  await nextTick();
  render();
});
onMounted(() => {
  render();
  window.addEventListener("resize", resize);
});
onBeforeUnmount(() => {
  window.removeEventListener("resize", resize);
  chart?.dispose();
});
</script>

<template>
  <div v-if="hasTrafficData(remote, cached, failed)">
    <div
      ref="chartElement"
      class="h-72 w-full"
      role="img"
      :aria-label="`请求结构：远端请求 ${remote}，缓存命中 ${cached}，失败请求 ${failed}`"
    />
  </div>
  <div v-else class="grid h-72 place-items-center text-center">
    <div>
      <p class="font-semibold">暂无请求数据</p>
      <p class="mt-2 text-sm text-muted-foreground">用户开始刷新设备后，这里会显示请求结构。</p>
    </div>
  </div>
</template>
