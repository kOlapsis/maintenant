<script setup lang="ts">
import { computed, ref } from 'vue'
import type { StateTransition } from '@/services/containerApi'

const props = withDefaults(defineProps<{
  transitions: StateTransition[]
  hours?: number
}>(), {
  hours: 24,
})

const tooltip = ref<{ visible: boolean; x: number; y: number; transition: StateTransition | null }>({
  visible: false,
  x: 0,
  y: 0,
  transition: null,
})

const timeWindow = computed(() => {
  const now = new Date()
  const start = new Date(now.getTime() - props.hours * 60 * 60 * 1000)
  return { start, end: now }
})

const filteredTransitions = computed(() => {
  const start = timeWindow.value.start.getTime()
  return props.transitions.filter(t => new Date(t.timestamp).getTime() >= start)
})

function positionPercent(timestamp: string): number {
  const ts = new Date(timestamp).getTime()
  const start = timeWindow.value.start.getTime()
  const end = timeWindow.value.end.getTime()
  const pct = ((ts - start) / (end - start)) * 100
  return Math.max(0, Math.min(100, pct))
}

function nowPercent(): number {
  return 100
}

function eventColor(t: StateTransition): string {
  const state = t.new_state
  if (state === 'running') return 'var(--pb-status-ok)'
  if (state === 'exited' || state === 'dead') return 'var(--pb-status-down)'
  if (state === 'restarting') return 'var(--pb-status-warn)'
  if (state === 'paused') return 'var(--pb-status-paused)'
  return 'var(--pb-text-muted)'
}

function showTooltip(event: MouseEvent, t: StateTransition) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  tooltip.value = {
    visible: true,
    x: rect.left + rect.width / 2,
    y: rect.top,
    transition: t,
  }
}

function hideTooltip() {
  tooltip.value.visible = false
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString()
}

const timeLabels = computed(() => {
  const labels: { text: string; pct: number }[] = []
  const intervals = Math.min(props.hours, 6)
  for (let i = 0; i <= intervals; i++) {
    const pct = (i / intervals) * 100
    const time = new Date(
      timeWindow.value.start.getTime() +
      (i / intervals) * (timeWindow.value.end.getTime() - timeWindow.value.start.getTime())
    )
    labels.push({
      text: time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      pct,
    })
  }
  return labels
})
</script>

<template>
  <div>
    <div class="mb-1 text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">
      Event Timeline ({{ hours }}h)
    </div>

    <!-- Timeline bar -->
    <div
      :style="{
        position: 'relative',
        height: '24px',
        backgroundColor: 'var(--pb-bg-elevated)',
        borderRadius: 'var(--pb-radius-md)',
        overflow: 'visible',
      }"
    >
      <!-- Event markers -->
      <div
        v-for="(t, i) in filteredTransitions"
        :key="i"
        :style="{
          position: 'absolute',
          left: positionPercent(t.timestamp) + '%',
          top: '50%',
          transform: 'translate(-50%, -50%)',
          width: '10px',
          height: '10px',
          borderRadius: '50%',
          backgroundColor: eventColor(t),
          border: '2px solid var(--pb-bg-surface)',
          cursor: 'pointer',
          zIndex: 2,
        }"
        @mouseenter="showTooltip($event, t)"
        @mouseleave="hideTooltip"
      />

      <!-- Current time indicator -->
      <div
        :style="{
          position: 'absolute',
          left: nowPercent() + '%',
          top: '0',
          width: '2px',
          height: '100%',
          backgroundColor: 'var(--pb-accent)',
          zIndex: 1,
        }"
      />
    </div>

    <!-- Time labels -->
    <div :style="{ position: 'relative', height: '16px', marginTop: '2px' }">
      <span
        v-for="label in timeLabels"
        :key="label.pct"
        :style="{
          position: 'absolute',
          left: label.pct + '%',
          transform: 'translateX(-50%)',
          fontSize: '0.625rem',
          color: 'var(--pb-text-muted)',
          whiteSpace: 'nowrap',
        }"
      >
        {{ label.text }}
      </span>
    </div>

    <!-- Tooltip -->
    <Teleport to="body">
      <div
        v-if="tooltip.visible && tooltip.transition"
        :style="{
          position: 'fixed',
          left: tooltip.x + 'px',
          top: (tooltip.y - 8) + 'px',
          transform: 'translate(-50%, -100%)',
          backgroundColor: 'var(--pb-bg-elevated)',
          color: 'var(--pb-text-primary)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-md)',
          padding: '0.5rem 0.75rem',
          fontSize: '0.75rem',
          boxShadow: 'var(--pb-shadow-elevated)',
          zIndex: 9999,
          pointerEvents: 'none',
          whiteSpace: 'nowrap',
        }"
      >
        <div :style="{ fontWeight: '600', marginBottom: '0.125rem' }">
          {{ tooltip.transition.previous_state }} &rarr; {{ tooltip.transition.new_state }}
        </div>
        <div :style="{ color: 'var(--pb-text-muted)' }">
          {{ formatTimestamp(tooltip.transition.timestamp) }}
        </div>
        <div v-if="tooltip.transition.exit_code !== undefined && tooltip.transition.exit_code !== null" :style="{ color: 'var(--pb-status-down)' }">
          Exit code: {{ tooltip.transition.exit_code }}
        </div>
      </div>
    </Teleport>
  </div>
</template>
