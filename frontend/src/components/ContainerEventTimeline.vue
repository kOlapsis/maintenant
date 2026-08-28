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
import type { StateTransition } from '@/services/containerApi'
import { getStateColor } from '@/utils/containerState'

const props = withDefaults(defineProps<{
  transitions: StateTransition[]
  hours?: number
  /** Number of bars to render */
  bars?: number
  /** Current container state — used as fallback when no transitions exist */
  currentState?: string
}>(), {
  hours: 24,
  bars: 48,
  currentState: 'running',
})

const tooltip = ref<{ visible: boolean; x: number; y: number; state: string; from: string; to: string } | null>(null)

const timeWindow = computed(() => {
  const now = new Date()
  const start = new Date(now.getTime() - props.hours * 60 * 60 * 1000)
  return { start: start.getTime(), end: now.getTime() }
})

function stateColor(state: string): string {
  return getStateColor(state)
}

function stateOpacity(state: string): string {
  if (state === 'running') return '0.4'
  return '1'
}

/** Build sorted transitions within window, prepending initial state */
const sortedTransitions = computed(() => {
  const { start, end } = timeWindow.value
  const all = props.transitions
    .map(t => ({ state: t.new_state, ts: new Date(t.timestamp).getTime() }))
    .filter(t => t.ts <= end)
    .sort((a, b) => a.ts - b.ts)

  // Determine state at window start: last transition before window start
  const before = all.filter(t => t.ts < start)
  const initialState = before.length > 0 ? before[before.length - 1]!.state : (all.length > 0 ? all[0]!.state : props.currentState)

  // Filter to only transitions within window
  const inWindow = all.filter(t => t.ts >= start)

  return { initialState, transitions: inWindow }
})

/** Generate bar data: each bar has a dominant state for its time slice */
const barData = computed(() => {
  const { start, end } = timeWindow.value
  const { initialState, transitions } = sortedTransitions.value
  const sliceDuration = (end - start) / props.bars
  const result: { state: string; sliceStart: number; sliceEnd: number }[] = []

  let tIdx = 0
  let currentState = initialState

  for (let i = 0; i < props.bars; i++) {
    const sliceStart = start + i * sliceDuration
    const sliceEnd = sliceStart + sliceDuration

    // Advance through transitions that fall within this slice
    let dominantState = currentState
    while (tIdx < transitions.length && transitions[tIdx]!.ts < sliceEnd) {
      dominantState = transitions[tIdx]!.state
      currentState = dominantState
      tIdx++
    }

    result.push({ state: dominantState, sliceStart, sliceEnd })
  }

  return result
})

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

function showBarTooltip(event: MouseEvent, bar: { state: string; sliceStart: number; sliceEnd: number }) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  tooltip.value = {
    visible: true,
    x: rect.left + rect.width / 2,
    y: rect.top,
    state: bar.state,
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

    <!-- Segmented bar -->
    <div class="flex gap-[2px] items-center h-5">
      <div
        v-for="(bar, i) in barData"
        :key="i"
        class="h-4 w-full rounded-sm transition-opacity cursor-help"
        :style="{
          backgroundColor: stateColor(bar.state),
          opacity: stateOpacity(bar.state),
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
        <div :style="{ fontWeight: '600', marginBottom: '0.125rem', color: stateColor(tooltip.state) }">
          {{ tooltip.state }}
        </div>
        <div :style="{ color: 'var(--mnt-text-muted)' }">
          {{ tooltip.from }} &mdash; {{ tooltip.to }}
        </div>
      </div>
    </Teleport>
  </div>
</template>
