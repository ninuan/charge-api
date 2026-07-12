<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { TrendingUp } from "@lucide/vue";
import type { MetricPoint } from "@/types/dashboard";

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{ hourly: MetricPoint[]; daily: MetricPoint[] }>();
type TrendRange = "24h" | "7d" | "30d";
const range = ref<TrendRange>("24h");
const chartElement = ref<HTMLDivElement | null>(null);
let chart: EChartsType | null = null;

const points = computed(() => range.value === "24h"
  ? props.hourly.slice(-24)
  : props.daily.slice(range.value === "7d" ? -7 : -30));
const hasData = computed(() => points.value.some((point) =>
  point.remoteOk + point.remoteFailed + point.cacheHits > 0
));
const summary = computed(() => points.value.reduce((result, point) => {
  result.remoteOk += point.remoteOk;
  result.remoteFailed += point.remoteFailed;
  result.cacheHits += point.cacheHits;
  return result;
}, { remoteOk: 0, remoteFailed: 0, cacheHits: 0 }));

function render() {
  if (!chartElement.value || !hasData.value) {
    chart?.dispose();
    chart = null;
    return;
  }
  chart ??= init(chartElement.value);
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  chart.setOption({
    animationDuration: reduceMotion ? 0 : 250,
    color: ["#21835b", "#d15a4b", "#d8992e"],
    tooltip: {
      trigger: "axis",
      backgroundColor: "rgba(255,255,255,.96)",
      borderColor: "#d9e3de",
      textStyle: { color: "#17211c" }
    },
    legend: { top: 2, textStyle: { color: "#68736d", fontSize: 11 }, itemWidth: 15, itemHeight: 7 },
    grid: { left: 8, right: 12, top: 44, bottom: 14, containLabel: true },
    xAxis: {
      type: "category",
      boundaryGap: points.value.length === 1,
      axisLine: { lineStyle: { color: "#d9e3de" } },
      axisTick: { show: false },
      axisLabel: { color: "#68736d", hideOverlap: true },
      data: points.value.map((point) => {
        const date = new Date(point.time);
        return range.value === "24h" ? `${date.getHours()}时` : `${date.getMonth() + 1}/${date.getDate()}`;
      })
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: "rgba(104,115,109,.12)" } },
      axisLabel: { color: "#68736d" }
    },
    series: [
      {
        name: "远端成功", type: "line", smooth: true, symbol: "circle", symbolSize: 7,
        lineStyle: { width: 3 }, areaStyle: points.value.length > 1 ? { opacity: .08 } : undefined,
        data: points.value.map((point) => point.remoteOk)
      },
      {
        name: "远端失败", type: "line", smooth: true, symbol: "circle", symbolSize: 7,
        lineStyle: { width: 2, type: "dashed" },
        data: points.value.map((point) => point.remoteFailed)
      },
      {
        name: "缓存命中", type: "line", smooth: true, symbol: "emptyCircle", symbolSize: 7,
        lineStyle: { width: 2 },
        data: points.value.map((point) => point.cacheHits)
      }
    ]
  });
}

function resize() {
  chart?.resize();
}

watch([points, hasData], async () => {
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
  <article class="admin-trend-panel surface-panel overflow-hidden">
    <header class="admin-overview-panel-head">
      <div class="admin-overview-panel-title">
        <TrendingUp aria-hidden="true" />
        <span>
          <h2>请求与成功率趋势</h2>
          <small>远端成功、失败与缓存命中</small>
        </span>
      </div>
      <div class="admin-range-switch" aria-label="趋势时间范围">
        <button v-for="item in ([
          { value: '24h', label: '24小时' },
          { value: '7d', label: '7天' },
          { value: '30d', label: '30天' }
        ] as const)" :key="item.value" type="button" :aria-pressed="range === item.value" @click="range = item.value">
          {{ item.label }}
        </button>
      </div>
    </header>
    <div v-if="hasData" class="admin-trend-body">
      <div
        ref="chartElement"
        class="admin-trend-chart w-full"
        role="img"
        :aria-label="`请求趋势：远端成功 ${summary.remoteOk}，远端失败 ${summary.remoteFailed}，缓存命中 ${summary.cacheHits}`"
      />
    </div>
    <div v-else class="empty-compact min-h-64">暂无趋势数据，用户刷新设备后将在这里显示。</div>
  </article>
</template>
