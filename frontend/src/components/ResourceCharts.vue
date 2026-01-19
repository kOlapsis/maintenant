<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { getResourceHistory, type HistoryPoint } from '@/services/resourceApi'
import { useChart } from '@/composables/useChart'
import { useResourcesStore } from '@/stores/resources'
import type uPlot from 'uplot'

const props = defineProps<{
  containerId: number
}>()

const resourcesStore = useResourcesStore()
const selectedRange = ref('1h')
const ranges = ['1h', '6h', '24h', '7d']
const loading = ref(false)
const points = ref<HistoryPoint[]>([])

const cpuEl = ref<HTMLElement | null>(null)
const memEl = ref<HTMLElement | null>(null)
const netEl = ref<HTMLElement | null>(null)
const ioEl = ref<HTMLElement | null>(null)

function toTimestamps(pts: HistoryPoint[]): number[] {
  return pts.map((p) => new Date(p.timestamp).getTime() / 1000)
}

// Use design-token-aligned colors
const chartColors = ['#3b82f6', '#22c55e', '#eab308', '#ef4444']

function bytesAxisFormatter(rawValue: number): string {
  return resourcesStore.formatBytes(rawValue)
}

function pctAxisFormatter(rawValue: number): string {
  return rawValue.toFixed(0) + '%'
}

const cpuChart = useChart({
  el: cpuEl,
  opts: () => ({
    height: 180,
    scales: { x: { time: true }, y: { auto: true } },
    axes: [
      {},
      { values: (_u: uPlot, vals: number[]) => vals.map(pctAxisFormatter) },
    ],
    series: [
      {},
      { label: 'CPU %', stroke: chartColors[0], width: 2, fill: chartColors[0] + '20' },
    ],
  }),
  data: () => [[], []] as uPlot.AlignedData,
})

const memChart = useChart({
  el: memEl,
  opts: () => ({
    height: 180,
    scales: { x: { time: true }, y: { auto: true } },
    axes: [
      {},
      { values: (_u: uPlot, vals: number[]) => vals.map(bytesAxisFormatter) },
    ],
    series: [
      {},
      { label: 'Memory', stroke: chartColors[1], width: 2, fill: chartColors[1] + '20' },
    ],
  }),
  data: () => [[], []] as uPlot.AlignedData,
})

const netChart = useChart({
  el: netEl,
  opts: () => ({
    height: 180,
    scales: { x: { time: true }, y: { auto: true } },
    axes: [
      {},
      { values: (_u: uPlot, vals: number[]) => vals.map(bytesAxisFormatter) },
    ],
    series: [
      {},
      { label: 'RX', stroke: chartColors[0], width: 2 },
      { label: 'TX', stroke: chartColors[2], width: 2 },
    ],
  }),
  data: () => [[], [], []] as uPlot.AlignedData,
})

const ioChart = useChart({
  el: ioEl,
  opts: () => ({
    height: 180,
    scales: { x: { time: true }, y: { auto: true } },
    axes: [
      {},
      { values: (_u: uPlot, vals: number[]) => vals.map(bytesAxisFormatter) },
    ],
    series: [
      {},
      { label: 'Read', stroke: chartColors[1], width: 2 },
      { label: 'Write', stroke: chartColors[3], width: 2 },
    ],
  }),
  data: () => [[], [], []] as uPlot.AlignedData,
})

async function fetchHistory() {
  loading.value = true
  try {
    const res = await getResourceHistory(props.containerId, selectedRange.value)
    points.value = res.points || []
    updateCharts()
  } catch {
    points.value = []
  } finally {
    loading.value = false
  }
}

function updateCharts() {
  const ts = toTimestamps(points.value)
  cpuChart.setData([ts, points.value.map((p) => p.cpu_percent)])
  memChart.setData([ts, points.value.map((p) => p.mem_used)])
  netChart.setData([
    ts,
    points.value.map((p) => p.net_rx_bytes),
    points.value.map((p) => p.net_tx_bytes),
  ])
  ioChart.setData([
    ts,
    points.value.map((p) => p.block_read_bytes),
    points.value.map((p) => p.block_write_bytes),
  ])
}

watch(selectedRange, () => fetchHistory())
onMounted(() => fetchHistory())
</script>

<template>
  <div class="space-y-4">
    <!-- Range selector -->
    <div class="flex items-center gap-2">
      <span class="text-sm font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Time Range:</span>
      <div
        class="flex"
        :style="{
          borderRadius: 'var(--pb-radius-md)',
          border: '1px solid var(--pb-border-default)',
          overflow: 'hidden',
        }"
      >
        <button
          v-for="r in ranges"
          :key="r"
          class="px-3 py-1 text-xs font-medium transition"
          :style="{
            backgroundColor: selectedRange === r ? 'var(--pb-accent)' : 'var(--pb-bg-surface)',
            color: selectedRange === r ? 'var(--pb-text-inverted)' : 'var(--pb-text-secondary)',
          }"
          @click="selectedRange = r"
        >
          {{ r }}
        </button>
      </div>
      <div
        v-if="loading"
        class="ml-2 h-4 w-4 animate-spin rounded-full border-2"
        :style="{ borderColor: 'var(--pb-border-default)', borderTopColor: 'var(--pb-accent)' }"
      />
    </div>

    <!-- Empty state -->
    <div
      v-if="!loading && points.length === 0"
      class="rounded p-6 text-center text-sm"
      :style="{
        backgroundColor: 'var(--pb-bg-elevated)',
        border: '1px solid var(--pb-border-subtle)',
        color: 'var(--pb-text-muted)',
        borderRadius: 'var(--pb-radius-md)',
      }"
    >
      No resource data available for this time range.
    </div>

    <!-- Charts -->
    <div v-else class="grid gap-4 md:grid-cols-2">
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">CPU Usage</h4>
        <div ref="cpuEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Memory Usage</h4>
        <div ref="memEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Network I/O</h4>
        <div ref="netEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Block I/O</h4>
        <div ref="ioEl" class="w-full" />
      </div>
    </div>
  </div>
</template>
