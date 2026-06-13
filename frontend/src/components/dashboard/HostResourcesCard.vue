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
import { computed, onMounted, onUnmounted } from 'vue'
import { useResourcesStore } from '@/stores/resources'
import { useDashboardStore } from '@/stores/dashboard'
import { Server } from 'lucide-vue-next'

// showMonitorStats: the Monitors/Availability rows read the dashboard store,
// which only the dashboard page populates — hide them elsewhere.
const props = withDefaults(defineProps<{ showMonitorStats?: boolean }>(), {
  showMonitorStats: true,
})

const resources = useResourcesStore()
const dashboard = useDashboardStore()

const totalCpu = computed(() => resources.summary?.total_cpu_percent ?? 0)
const totalMemUsed = computed(() => resources.summary?.total_mem_used ?? 0)
const totalMemLimit = computed(() => resources.summary?.total_mem_limit ?? 0)
const memPercent = computed(() => {
  if (totalMemLimit.value === 0) return 0
  return (totalMemUsed.value / totalMemLimit.value) * 100
})

function gaugeBarColor(val: number, thresholds = { warn: 60, crit: 80 }): string {
  if (val > thresholds.crit) return 'bg-pb-sev-incident-solid'
  if (val > thresholds.warn) return 'bg-pb-sev-warning-solid'
  return 'bg-pb-green-500'
}

let summaryTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  resources.fetchSummary()
  summaryTimer = setInterval(() => resources.fetchSummary(), 3_000)
})

onUnmounted(() => {
  if (summaryTimer) clearInterval(summaryTimer)
})
</script>

<template>
  <div class="bg-pb-surface rounded-xl sm:rounded-2xl border border-pb-default p-4 sm:p-6">
    <div class="flex items-center gap-2.5 mb-5">
      <Server :size="15" class="text-pb-status-ok" />
      <h3 class="text-sm font-bold text-pb-primary">Host Resources</h3>
    </div>

    <div class="space-y-5">
      <!-- CPU -->
      <div class="space-y-1.5">
        <div class="flex justify-between items-center text-[10px] font-bold uppercase tracking-widest">
          <span class="text-pb-muted">CPU Usage</span>
          <span class="text-pb-primary">{{ Math.round(totalCpu) }}%</span>
        </div>
        <div class="h-1.5 w-full bg-pb-primary rounded-full border border-pb-default overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-700"
            :class="gaugeBarColor(totalCpu)"
            :style="{ width: `${Math.min(totalCpu, 100)}%` }"
          />
        </div>
      </div>

      <!-- RAM -->
      <div class="space-y-1.5">
        <div class="flex justify-between items-center text-[10px] font-bold uppercase tracking-widest">
          <span class="text-pb-muted">RAM Memory</span>
          <span class="text-pb-primary text-right">
            {{ resources.formatBytes(totalMemUsed) }} / {{ resources.formatBytes(totalMemLimit) }}
          </span>
        </div>
        <div class="h-1.5 w-full bg-pb-primary rounded-full border border-pb-default overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-700"
            :class="gaugeBarColor(memPercent, { warn: 70, crit: 85 })"
            :style="{ width: `${Math.min(memPercent, 100)}%` }"
          />
        </div>
      </div>

      <!-- Stats -->
      <div class="pt-4 border-t border-pb-default space-y-2.5">
        <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
          <span class="text-pb-muted">Containers</span>
          <span class="text-pb-secondary font-mono">{{ Object.keys(resources.snapshots).length }} active</span>
        </div>
        <template v-if="props.showMonitorStats">
          <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
            <span class="text-pb-muted">Monitors</span>
            <span class="text-pb-secondary font-mono">{{ dashboard.monitors.length }} total</span>
          </div>
          <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
            <span class="text-pb-muted">Availability</span>
            <span class="text-pb-status-ok font-mono">
              {{ dashboard.monitors.length > 0 ? ((dashboard.globalStats.running / dashboard.monitors.length) * 100).toFixed(1) : '—' }}%
            </span>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
