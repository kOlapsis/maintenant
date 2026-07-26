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
import { computed, ref } from 'vue'
import type { CheckResult } from '@/services/endpointApi'

type EpStatus = 'up' | 'down' | 'degraded' | 'unknown'

const props = withDefaults(defineProps<{
  checks: CheckResult[]
  /** Window size in hours */
  hours?: number
  /** Number of bars to render */
  bars?: number
  /** Current status — carried forward into slices with no check */
  currentStatus?: EpStatus
}>(), {
  hours: 24,
  bars: 48,
  currentStatus: 'unknown',
})

const tooltip = ref<{ visible: boolean; x: number; y: number; status: EpStatus; from: string; to: string } | null>(null)

const statusColors: Record<EpStatus, string> = {
  up: 'var(--mnt-status-ok)',
  down: 'var(--mnt-status-down)',
  degraded: 'var(--mnt-status-warn)',
  unknown: 'var(--mnt-sev-unknown)',
}

function statusColor(status: EpStatus): string {
  return statusColors[status]
}

function statusOpacity(status: EpStatus): string {
  if (status === 'up') return '0.5'
  if (status === 'unknown') return '0.35'
  return '1'
}

function statusLabel(status: EpStatus): string {
  if (status === 'up') return 'Up'
  if (status === 'down') return 'Down'
  if (status === 'degraded') return 'Degraded'
  return 'No data'
}

const timeWindow = computed(() => {
  const now = new Date()
  const start = new Date(now.getTime() - props.hours * 60 * 60 * 1000)
  return { start: start.getTime(), end: now.getTime() }
})

/** Checks sorted ascending, with epoch timestamps. */
const sortedChecks = computed(() => {
  return props.checks
    .map(c => ({ ts: new Date(c.timestamp).getTime(), success: c.success }))
    .filter(c => !Number.isNaN(c.ts))
    .sort((a, b) => a.ts - b.ts)
})

/**
 * Build bar data: each bar is the dominant status of its slice. A slice with any
 * failure is `down` (Uptime-Kuma "down dominates"), any success-only slice is `up`,
 * and an empty slice carries forward the last known status (or `unknown`).
 */
const barData = computed(() => {
  const { start, end } = timeWindow.value
  const checks = sortedChecks.value
  const sliceDuration = (end - start) / props.bars
  const result: { status: EpStatus; sliceStart: number; sliceEnd: number }[] = []

  // Status entering the window: last check before the window, else currentStatus.
  const before = checks.filter(c => c.ts < start)
  let lastStatus: EpStatus = before.length > 0
    ? (before[before.length - 1]!.success ? 'up' : 'down')
    : props.currentStatus

  let cIdx = checks.findIndex(c => c.ts >= start)
  if (cIdx === -1) cIdx = checks.length

  for (let i = 0; i < props.bars; i++) {
    const sliceStart = start + i * sliceDuration
    const sliceEnd = sliceStart + sliceDuration

    let anyCheck = false
    let anyFailure = false
    while (cIdx < checks.length && checks[cIdx]!.ts < sliceEnd) {
      anyCheck = true
      if (!checks[cIdx]!.success) anyFailure = true
      cIdx++
    }

    let status: EpStatus
    if (anyCheck) {
      status = anyFailure ? 'down' : 'up'
      lastStatus = status
    } else {
      status = lastStatus
    }

    result.push({ status, sliceStart, sliceEnd })
  }

  return result
})

const isEmpty = computed(() => props.checks.length === 0)

const timeLabels = computed(() => {
  const labels: { text: string; pct: number }[] = []
  const intervals = Math.min(props.hours, 6)
  const { start, end } = timeWindow.value
  for (let i = 0; i <= intervals; i++) {
    const pct = (i / intervals) * 100
    const time = new Date(start + (i / intervals) * (end - start))
    labels.push({
      text: time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      pct,
    })
  }
  return labels
})

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function showBarTooltip(event: MouseEvent, bar: { status: EpStatus; sliceStart: number; sliceEnd: number }) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  tooltip.value = {
    visible: true,
    x: rect.left + rect.width / 2,
    y: rect.top,
    status: bar.status,
    from: formatTime(bar.sliceStart),
    to: formatTime(bar.sliceEnd),
  }
}

function hideTooltip() {
  tooltip.value = null
}
</script>

<template>
  <div>
    <div class="mb-1 text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">
      Event Timeline ({{ hours }}h)
    </div>

    <!-- Empty state -->
    <div
      v-if="isEmpty"
      class="flex h-5 items-center text-xs"
      :style="{ color: 'var(--mnt-text-muted)' }"
    >
      No check history yet.
    </div>

    <template v-else>
      <!-- Segmented bar -->
      <div class="flex gap-[2px] items-center h-5">
        <div
          v-for="(bar, i) in barData"
          :key="i"
          class="h-4 w-full rounded-sm transition-opacity cursor-help"
          :style="{
            backgroundColor: statusColor(bar.status),
            opacity: statusOpacity(bar.status),
            flex: '1 1 0',
            minWidth: '2px',
          }"
          @mouseenter="showBarTooltip($event, bar)"
          @mouseleave="hideTooltip"
        />
      </div>

      <!-- Time labels -->
      <div :style="{ position: 'relative', height: '16px', marginTop: '4px' }">
        <span
          v-for="label in timeLabels"
          :key="label.pct"
          :style="{
            position: 'absolute',
            left: label.pct + '%',
            transform: 'translateX(-50%)',
            fontSize: '0.625rem',
            color: 'var(--mnt-text-muted)',
            whiteSpace: 'nowrap',
          }"
        >
          {{ label.text }}
        </span>
      </div>
    </template>

    <!-- Tooltip -->
    <Teleport to="body">
      <div
        v-if="tooltip?.visible"
        :style="{
          position: 'fixed',
          left: tooltip.x + 'px',
          top: (tooltip.y - 8) + 'px',
          transform: 'translate(-50%, -100%)',
          backgroundColor: 'var(--mnt-bg-elevated)',
          color: 'var(--mnt-text-primary)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-md)',
          padding: '0.5rem 0.75rem',
          fontSize: '0.75rem',
          boxShadow: 'var(--mnt-shadow-elevated)',
          zIndex: 9999,
          pointerEvents: 'none',
          whiteSpace: 'nowrap',
        }"
      >
        <div :style="{ fontWeight: '600', marginBottom: '0.125rem', color: statusColor(tooltip.status) }">
          {{ statusLabel(tooltip.status) }}
        </div>
        <div :style="{ color: 'var(--mnt-text-muted)' }">
          {{ tooltip.from }} &mdash; {{ tooltip.to }}
        </div>
      </div>
    </Teleport>
  </div>
</template>
