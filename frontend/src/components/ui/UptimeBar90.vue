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
import { ref } from 'vue'
import type { UptimeDay } from '@/services/uptimeApi'

const props = withDefaults(defineProps<{
  days: UptimeDay[]
  compact?: boolean
}>(), {
  compact: false,
})

const tooltip = ref<{ visible: boolean; x: number; y: number; day: UptimeDay | null }>({
  visible: false,
  x: 0,
  y: 0,
  day: null,
})

function barColorClass(day: UptimeDay): string {
  if (day.uptime_percent === null) return 'bg-slate-700'
  if (day.uptime_percent >= 100) return 'bg-emerald-500'
  if (day.uptime_percent > 0) return 'bg-amber-500'
  return 'bg-rose-500'
}

function showTooltip(event: MouseEvent, day: UptimeDay) {
  if (props.compact) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  tooltip.value = {
    visible: true,
    x: rect.left + rect.width / 2,
    y: rect.top,
    day,
  }
}

function hideTooltip() {
  tooltip.value.visible = false
}

function formatDate(dateStr: string): string {
  const d = new Date(dateStr)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function formatUptime(pct: number | null): string {
  if (pct === null) return 'No data'
  return `${pct.toFixed(2)}%`
}
</script>

<template>
  <div class="relative">
    <div class="flex items-center gap-px" :style="{ height: compact ? '16px' : '28px' }">
      <div
        v-for="(day, i) in days"
        :key="i"
        class="h-4 w-[2px] rounded-full transition-opacity hover:opacity-80"
        :class="barColorClass(day)"
        :style="{ cursor: compact ? 'default' : 'pointer' }"
        @mouseenter="showTooltip($event, day)"
        @mouseleave="hideTooltip"
      />
    </div>

    <!-- Tooltip -->
    <Teleport to="body">
      <div
        v-if="tooltip.visible && tooltip.day && !compact"
        class="fixed z-[9999] pointer-events-none whitespace-nowrap bg-pb-surface text-pb-primary border border-slate-800 rounded-lg px-3 py-2 text-xs shadow-xl"
        :style="{
          left: tooltip.x + 'px',
          top: (tooltip.y - 8) + 'px',
          transform: 'translate(-50%, -100%)',
        }"
      >
        <div class="font-semibold mb-0.5">
          {{ formatDate(tooltip.day.date) }}
        </div>
        <div>Uptime: {{ formatUptime(tooltip.day.uptime_percent) }}</div>
        <div class="text-slate-500">
          {{ tooltip.day.incident_count }} incident{{ tooltip.day.incident_count !== 1 ? 's' : '' }}
        </div>
      </div>
    </Teleport>
  </div>
</template>
