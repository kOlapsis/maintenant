<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { getResourceHistory, type HistoryPoint } from '@/services/resourceApi'
import { useChart } from '@/composables/useChart'
import { useResourcesStore } from '@/stores/resources'
import { useEdition } from '@/composables/useEdition'
import { ApiError } from '@/services/apiFetch'
import EditionBadge from '@/components/EditionBadge.vue'
import { Lock } from 'lucide-vue-next'
import type uPlot from 'uplot'

const props = defineProps<{
  containerId: string
}>()

const resourcesStore = useResourcesStore()
const { historyWindows, isWindowOpen, requiredEditionForWindow, largestOpenWindow, reload } =
  useEdition()
const selectedRange = ref('1h')
const loading = ref(false)
const points = ref<HistoryPoint[]>([])
const closedNotice = ref('')

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

const charts = [cpuChart, memChart, netChart, ioChart]

async function fetchHistory() {
  loading.value = true
  try {
    const res = await getResourceHistory(props.containerId, selectedRange.value)
    points.value = res.points || []
  } catch (err) {
    points.value = []
    // A license can expire between two refreshes, and the view must not break
    // on the window it was already showing. The refusal itself is the signal:
    // fall back to the largest window still open and say so (FR-021b).
    if (err instanceof ApiError && err.isEditionRefusal) {
      const closed = selectedRange.value
      await reload()
      const fallback = largestOpenWindow.value
      if (fallback && fallback !== closed) {
        closedNotice.value = `${closed} is no longer available on this edition. Showing ${fallback}.`
        selectedRange.value = fallback
      }
    }
  } finally {
    loading.value = false
  }
}

// The chart elements sit behind the v-else, so they do not exist until there is
// data to show: uPlot has to be created once they are in the DOM, and torn down
// again when a range comes back empty and the branch is swapped out.
watch(points, async (pts) => {
  await nextTick()
  if (!pts.length) {
    charts.forEach((c) => c.destroy())
    return
  }
  charts.forEach((c) => {
    if (!c.ready.value) c.create()
  })
  updateCharts()
})

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

/**
 * A closed window is shown, never hidden: seeing one hour of history is what
 * makes thirty days worth wanting. Clicking it does nothing and asks nothing:
 * the interface does not fire a request it knows will be refused (FR-018).
 */
/** The cheapest window the running edition does not open, or null when it opens everything. */
const firstClosedWindow = computed(() => historyWindows.value.find((w) => !isWindowOpen(w.window)) ?? null)

function selectRange(window: string) {
  if (!isWindowOpen(window)) return
  // The fallback notice stands until the user picks a window themselves; the
  // refetch that follows the fallback must not erase what it just said.
  closedNotice.value = ''
  selectedRange.value = window
}

watch(selectedRange, () => fetchHistory())
onMounted(() => fetchHistory())
</script>

<template>
  <div class="space-y-4">
    <!-- Range selector -->
    <div class="flex items-center gap-2">
      <span class="text-sm font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Time Range:</span>
      <div
        class="flex"
        :style="{
          borderRadius: 'var(--mnt-radius-md)',
          border: '1px solid var(--mnt-border-default)',
          overflow: 'hidden',
        }"
      >
        <button
          v-for="w in historyWindows"
          :key="w.window"
          :data-test="`range-${w.window}`"
          :data-locked="isWindowOpen(w.window) ? undefined : 'true'"
          :disabled="!isWindowOpen(w.window)"
          class="flex items-center gap-1 px-3 py-1 text-xs font-medium transition"
          :style="{
            backgroundColor: selectedRange === w.window ? 'var(--mnt-accent)' : 'var(--mnt-bg-surface)',
            color: selectedRange === w.window
              ? 'var(--mnt-text-inverted)'
              : isWindowOpen(w.window)
                ? 'var(--mnt-text-secondary)'
                : 'var(--mnt-text-muted)',
            cursor: isWindowOpen(w.window) ? 'pointer' : 'not-allowed',
            opacity: isWindowOpen(w.window) ? '1' : '0.6',
          }"
          @click="selectRange(w.window)"
        >
          {{ w.window }}
          <Lock v-if="!isWindowOpen(w.window)" class="h-3 w-3" />
        </button>
      </div>
      <div
        v-if="loading"
        class="ml-2 h-4 w-4 animate-spin rounded-full border-2"
        :style="{ borderColor: 'var(--mnt-border-default)', borderTopColor: 'var(--mnt-accent)' }"
      />

      <!-- What the next window up would cost, in the vocabulary the rest of the
           interface already uses for gating. -->
      <router-link
        v-if="firstClosedWindow"
        :to="{ name: 'editions' }"
        class="ml-1 flex items-center gap-1.5 text-xs"
        :style="{ color: 'var(--mnt-text-muted)' }"
      >
        <Lock class="h-3 w-3" />
        <span>{{ firstClosedWindow.window }} and beyond</span>
        <EditionBadge
          v-if="requiredEditionForWindow(firstClosedWindow.window)"
          :edition="requiredEditionForWindow(firstClosedWindow.window)!"
        />
      </router-link>
    </div>

    <!-- The window closed under the view: said plainly, not as a failure. -->
    <div
      v-if="closedNotice"
      class="rounded px-3 py-2 text-xs"
      :style="{
        backgroundColor: 'var(--mnt-bg-elevated)',
        border: '1px solid var(--mnt-border-subtle)',
        color: 'var(--mnt-text-secondary)',
        borderRadius: 'var(--mnt-radius-md)',
      }"
      data-test="window-closed-notice"
    >
      {{ closedNotice }}
    </div>

    <!-- Empty state -->
    <div
      v-if="!loading && points.length === 0"
      class="rounded p-6 text-center text-sm"
      :style="{
        backgroundColor: 'var(--mnt-bg-elevated)',
        border: '1px solid var(--mnt-border-subtle)',
        color: 'var(--mnt-text-muted)',
        borderRadius: 'var(--mnt-radius-md)',
      }"
    >
      No resource data available for this time range.
    </div>

    <!-- Charts -->
    <div v-else class="grid gap-4 md:grid-cols-2">
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">CPU Usage</h4>
        <div ref="cpuEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">Memory Usage</h4>
        <div ref="memEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">Network I/O</h4>
        <div ref="netEl" class="w-full" />
      </div>
      <div
        class="rounded p-3"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
      >
        <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">Block I/O</h4>
        <div ref="ioEl" class="w-full" />
      </div>
    </div>
  </div>
</template>
