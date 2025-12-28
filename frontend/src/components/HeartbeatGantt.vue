<script setup lang="ts">
import { computed } from 'vue'

export interface HeartbeatTimelineEntry {
  expectedAt: string
  receivedAt: string | null
  graceDeadline: string
  status: 'on-time' | 'late' | 'missed'
  duration: number | null
}

const props = withDefaults(defineProps<{
  entries: HeartbeatTimelineEntry[]
  hours: number
}>(), {
  hours: 48,
})

const statusColors: Record<string, string> = {
  'on-time': 'var(--pb-status-ok)',
  'late': 'var(--pb-status-warn)',
  'missed': 'var(--pb-status-down)',
}

const statusBgColors: Record<string, string> = {
  'on-time': 'var(--pb-status-ok-bg)',
  'late': 'var(--pb-status-warn-bg)',
  'missed': 'var(--pb-status-down-bg)',
}

const timeRange = computed(() => {
  const end = Date.now()
  const start = end - props.hours * 60 * 60 * 1000
  return { start, end, span: end - start }
})

const timeLabels = computed(() => {
  const labels: { time: string; pct: number }[] = []
  const { start, span } = timeRange.value
  // Generate labels every 6 hours for 48h, every 3 hours for 24h
  const intervalHours = props.hours > 24 ? 6 : 3
  const intervalMs = intervalHours * 60 * 60 * 1000

  // Align to interval boundary
  const firstLabel = Math.ceil(start / intervalMs) * intervalMs
  for (let t = firstLabel; t <= timeRange.value.end; t += intervalMs) {
    const pct = ((t - start) / span) * 100
    const d = new Date(t)
    labels.push({
      time: d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      pct,
    })
  }
  return labels
})

function toPercent(isoTime: string): number {
  const { start, span } = timeRange.value
  const t = new Date(isoTime).getTime()
  return Math.max(0, Math.min(100, ((t - start) / span) * 100))
}

function windowWidth(entry: HeartbeatTimelineEntry): number {
  const startPct = toPercent(entry.expectedAt)
  const endPct = toPercent(entry.graceDeadline)
  return Math.max(0.5, endPct - startPct)
}

function formatTooltip(entry: HeartbeatTimelineEntry): string {
  const expected = new Date(entry.expectedAt).toLocaleString()
  const received = entry.receivedAt ? new Date(entry.receivedAt).toLocaleString() : 'Not received'
  const dur = entry.duration !== null ? `${entry.duration}ms` : '-'
  return `Expected: ${expected}\nReceived: ${received}\nDuration: ${dur}\nStatus: ${entry.status}`
}
</script>

<template>
  <div class="w-full">
    <div class="mb-2 flex items-center gap-4">
      <h4 class="text-xs font-semibold uppercase tracking-wide" style="color: var(--pb-text-muted)">
        Ping Timeline ({{ hours }}h)
      </h4>
      <div class="flex items-center gap-3 text-xs" style="color: var(--pb-text-muted)">
        <span class="flex items-center gap-1">
          <span class="inline-block h-2 w-2 rounded-full" style="background: var(--pb-status-ok)"></span> On-time
        </span>
        <span class="flex items-center gap-1">
          <span class="inline-block h-2 w-2 rounded-full" style="background: var(--pb-status-warn)"></span> Late
        </span>
        <span class="flex items-center gap-1">
          <span class="inline-block h-2 w-2 rounded-full" style="background: var(--pb-status-down)"></span> Missed
        </span>
      </div>
    </div>

    <div
      class="relative rounded-lg border p-3"
      style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
    >
      <!-- Time axis labels -->
      <div class="relative mb-1 h-4">
        <span
          v-for="(label, idx) in timeLabels"
          :key="idx"
          class="absolute -translate-x-1/2 text-[10px]"
          :style="{ left: label.pct + '%', color: 'var(--pb-text-muted)' }"
        >
          {{ label.time }}
        </span>
      </div>

      <!-- Timeline track -->
      <div
        class="relative h-8 rounded"
        style="background: var(--pb-bg-elevated)"
      >
        <!-- Grid lines -->
        <span
          v-for="(label, idx) in timeLabels"
          :key="'grid-' + idx"
          class="absolute top-0 h-full w-px"
          :style="{ left: label.pct + '%', background: 'var(--pb-border-subtle)' }"
        />

        <!-- Expected windows (light bars) -->
        <div
          v-for="(entry, idx) in entries"
          :key="'window-' + idx"
          class="absolute top-1 h-6 rounded-sm"
          :style="{
            left: toPercent(entry.expectedAt) + '%',
            width: windowWidth(entry) + '%',
            background: statusBgColors[entry.status],
          }"
          :title="formatTooltip(entry)"
        />

        <!-- Actual ping markers -->
        <div
          v-for="(entry, idx) in entries"
          :key="'ping-' + idx"
          class="absolute top-1/2 -translate-x-1/2 -translate-y-1/2 cursor-pointer rounded-full transition-transform hover:scale-150"
          :style="{
            left: entry.receivedAt ? toPercent(entry.receivedAt) + '%' : toPercent(entry.graceDeadline) + '%',
            width: '8px',
            height: '8px',
            background: statusColors[entry.status],
            boxShadow: '0 0 4px ' + statusColors[entry.status],
          }"
          :title="formatTooltip(entry)"
        />
      </div>

      <!-- Now indicator -->
      <div
        class="absolute right-3 top-1/2 -translate-y-1/2"
        style="color: var(--pb-text-muted)"
      >
        <span class="text-[10px]">Now</span>
      </div>
    </div>

    <!-- Empty state -->
    <p
      v-if="entries.length === 0"
      class="mt-2 text-center text-xs"
      style="color: var(--pb-text-muted)"
    >
      No ping data available for this time period
    </p>
  </div>
</template>
