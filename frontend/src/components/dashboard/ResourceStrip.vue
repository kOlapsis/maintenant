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
import { computed } from 'vue'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import { Cpu, MemoryStick } from 'lucide-vue-next'

const resources = useResourcesStore()
const containers = useContainersStore()

const containerCount = computed(() => Object.keys(resources.snapshots).length)

const totalCpu = computed(() =>
  resources.summary?.total_cpu_percent ?? 0,
)

const totalMemUsed = computed(() =>
  resources.summary?.total_mem_used ?? 0,
)

const totalMemLimit = computed(() =>
  resources.summary?.total_mem_limit ?? 0,
)

const memPercent = computed(() => {
  if (totalMemLimit.value === 0) return 0
  return (totalMemUsed.value / totalMemLimit.value) * 100
})

// Resolve container names from containers store
function getContainerName(containerId: number): string {
  const c = containers.allContainers.find((ct) => ct.id === containerId)
  return c?.name || `container-${containerId}`
}

// Top 3 CPU consumers
const topConsumers = computed(() => {
  const entries = Object.entries(resources.snapshots)
  if (entries.length === 0) return []
  return entries
    .map(([idStr, snap]) => ({
      id: Number(idStr),
      name: getContainerName(Number(idStr)),
      cpu: snap.cpu_percent,
    }))
    .sort((a, b) => b.cpu - a.cpu)
    .slice(0, 3)
})

function cpuBarColor(val: number): string {
  if (val > 80) return 'bg-rose-500'
  if (val > 60) return 'bg-amber-500'
  return 'bg-pb-green-500'
}

function memBarColor(val: number): string {
  if (val > 85) return 'bg-rose-500'
  if (val > 70) return 'bg-amber-500'
  return 'bg-pb-green-500'
}


</script>

<template>
  <div
    v-if="containerCount > 0"
    class="mb-4 flex flex-wrap items-center gap-5 bg-pb-surface rounded-2xl border border-slate-800 px-5 py-3"
  >
    <!-- CPU gauge -->
    <div class="flex items-center gap-2.5 min-w-[160px]">
      <div class="p-1.5 rounded-lg" style="background: var(--pb-bg-elevated)">
        <Cpu :size="14" class="text-slate-500" />
      </div>
      <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">CPU</span>
      <div class="h-1.5 flex-1 rounded-full border border-slate-800 overflow-hidden" style="background: var(--pb-bg-elevated); min-width: 60px">
        <div
          class="h-full rounded-full transition-all duration-700"
          :class="cpuBarColor(totalCpu)"
          :style="{ width: `${Math.min(totalCpu, 100)}%` }"
        />
      </div>
      <span class="text-xs font-mono tabular-nums text-pb-primary font-semibold">{{ Math.round(totalCpu) }}%</span>
    </div>

    <!-- Memory gauge -->
    <div class="flex items-center gap-2.5 min-w-[200px]">
      <div class="p-1.5 rounded-lg" style="background: var(--pb-bg-elevated)">
        <MemoryStick :size="14" class="text-slate-500" />
      </div>
      <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">MEM</span>
      <div class="h-1.5 flex-1 rounded-full border border-slate-800 overflow-hidden" style="background: var(--pb-bg-elevated); min-width: 60px">
        <div
          class="h-full rounded-full transition-all duration-700"
          :class="memBarColor(memPercent)"
          :style="{ width: `${Math.min(memPercent, 100)}%` }"
        />
      </div>
      <span class="text-xs font-mono tabular-nums text-pb-primary font-semibold whitespace-nowrap">
        {{ resources.formatBytes(totalMemUsed) }} / {{ resources.formatBytes(totalMemLimit) }}
      </span>
    </div>

    <!-- Separator -->
    <div v-if="topConsumers.length > 0" class="hidden sm:block h-5 w-px bg-slate-800" />

    <!-- Top consumers -->
    <div v-if="topConsumers.length > 0" class="hidden sm:flex items-center gap-1.5">
      <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">Top:</span>
      <span
        v-for="(tc, i) in topConsumers"
        :key="tc.id"
        class="text-xs font-mono tabular-nums text-slate-400"
      >
        {{ tc.name }} {{ tc.cpu.toFixed(0) }}%<template v-if="i < topConsumers.length - 1">,</template>
      </span>
    </div>
  </div>
</template>
