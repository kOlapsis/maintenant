<script setup lang="ts">
import { RouterLink } from 'vue-router'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import SparklineChart from '@/components/ui/SparklineChart.vue'
import type { UnifiedMonitor } from '@/stores/dashboard'

defineProps<{
  monitor: UnifiedMonitor
}>()

const badgeStatusMap: Record<string, 'ok' | 'warning' | 'critical' | 'down' | 'paused' | 'unknown'> = {
  ok: 'ok',
  warning: 'warning',
  down: 'down',
  paused: 'paused',
  unknown: 'unknown',
}

const typeLabels: Record<string, string> = {
  container: 'Container',
  endpoint: 'Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'Certificate',
}
</script>

<template>
  <RouterLink
    :to="monitor.link"
    class="group flex items-center gap-3 px-3 hover:bg-slate-800/30 transition-all border-b border-slate-800/50"
    :style="{ padding: 'var(--pb-density-row-padding) 0.75rem' }"
  >
    <StatusBadge :status="badgeStatusMap[monitor.status] || 'unknown'" size="md" />

    <div class="flex-1 min-w-0">
      <div class="text-sm font-medium truncate text-white">
        {{ monitor.name }}
      </div>
      <div class="text-xs truncate text-slate-500">
        {{ monitor.subtitle }}
      </div>
    </div>

    <!-- Status label pill -->
    <span
      class="hidden sm:inline-flex shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
      :class="{
        'text-emerald-400 bg-emerald-500/10': monitor.status === 'ok',
        'text-amber-400 bg-amber-500/10': monitor.status === 'warning',
        'text-rose-400 bg-rose-500/10': monitor.status === 'down',
        'text-slate-400 bg-slate-800': monitor.status === 'paused' || monitor.status === 'unknown',
      }"
    >
      {{ monitor.statusLabel }}
    </span>

    <!-- Sparkline -->
    <SparklineChart
      v-if="monitor.sparklineData && monitor.sparklineData.length > 1"
      :data="monitor.sparklineData"
      :width="64"
      :height="20"
      class="hidden sm:block shrink-0"
    />

    <!-- Metric value -->
    <span
      v-if="monitor.metricValue"
      class="hidden md:inline-block shrink-0 text-xs font-mono tabular-nums min-w-[48px] text-right text-slate-400"
    >
      {{ monitor.metricValue }}
    </span>

    <!-- Type pill -->
    <span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-slate-900 text-slate-500">
      {{ typeLabels[monitor.type] || monitor.type }}
    </span>
  </RouterLink>
</template>
