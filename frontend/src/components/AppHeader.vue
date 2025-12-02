<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useDashboardStore } from '@/stores/dashboard'
import { useAlertsStore } from '@/stores/alerts'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import { Search, Bell, AlertTriangle } from 'lucide-vue-next'

const dashboard = useDashboardStore()
const alertsStore = useAlertsStore()
const resources = useResourcesStore()
const containers = useContainersStore()

let summaryInterval: ReturnType<typeof setInterval> | null = null

// Global SSE connections — always active while the app shell is mounted
onMounted(() => {
  containers.connectSSE()
  resources.connectSSE()
  resources.fetchSummary()
  summaryInterval = setInterval(() => resources.fetchSummary(), 30_000)
})

onUnmounted(() => {
  containers.disconnectSSE()
  resources.disconnectSSE()
  if (summaryInterval) clearInterval(summaryInterval)
})

const totalCpu = computed(() => {
  return Math.min(
    Object.values(resources.snapshots).reduce((sum, s) => sum + s.cpu_percent, 0),
    100,
  )
})

const memPercent = computed(() => {
  const used = Object.values(resources.snapshots).reduce((sum, s) => sum + s.mem_used, 0)
  const limit = Object.values(resources.snapshots).reduce((sum, s) => sum + s.mem_limit, 0)
  if (limit === 0) return 0
  return (used / limit) * 100
})

const diskPercent = computed(() => resources.summary?.disk_percent ?? 0)

function barColor(value: number): string {
  if (value >= 90) return '#f43f5e'
  if (value >= 70) return '#f59e0b'
  return '#10b981'
}
</script>

<template>
  <header class="hidden md:flex h-16 shrink-0 border-b border-slate-800 items-center justify-between px-6 bg-[#151923]/60 backdrop-blur-md z-10">
    <div class="flex items-center gap-5">
      <!-- Search -->
      <div class="relative group">
        <Search
          :size="15"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500 group-focus-within:text-blue-400 transition-colors"
        />
        <input
          v-model="dashboard.searchQuery"
          type="text"
          placeholder="Rechercher un service..."
          class="bg-[#0f1115] border border-slate-800 rounded-lg py-2 pl-9 pr-4 text-sm w-72 focus:outline-none focus:ring-1 focus:ring-blue-500/60 focus:border-blue-500/40 transition-all text-slate-200 placeholder:text-slate-600"
        />
      </div>

      <!-- Health counters -->
      <div class="hidden sm:flex items-center gap-5 border-l border-slate-800 pl-5">
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Running</span>
          <span class="text-sm font-black text-emerald-500">{{ dashboard.globalStats.running }}</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Incidents</span>
          <span
            class="text-sm font-black"
            :class="dashboard.globalStats.incidents > 0 ? 'text-rose-500' : 'text-slate-500'"
          >{{ dashboard.globalStats.incidents }}</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Warnings</span>
          <span
            class="text-sm font-black"
            :class="dashboard.globalStats.warnings > 0 ? 'text-amber-500' : 'text-slate-500'"
          >{{ dashboard.globalStats.warnings }}</span>
        </div>
      </div>

      <!-- Resource gauges -->
      <div class="hidden lg:flex items-center gap-4 border-l border-slate-800 pl-5">
        <!-- CPU -->
        <div class="flex items-center gap-2 min-w-[120px]">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest w-8">CPU</span>
          <div class="flex-1 h-1.5 rounded-full bg-slate-800 overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-500"
              :style="{ width: totalCpu + '%', backgroundColor: barColor(totalCpu) }"
            />
          </div>
          <span class="text-xs font-bold tabular-nums w-9 text-right" :style="{ color: barColor(totalCpu) }">
            {{ totalCpu.toFixed(0) }}%
          </span>
        </div>
        <!-- MEM -->
        <div class="flex items-center gap-2 min-w-[120px]">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest w-8">MEM</span>
          <div class="flex-1 h-1.5 rounded-full bg-slate-800 overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-500"
              :style="{ width: memPercent + '%', backgroundColor: barColor(memPercent) }"
            />
          </div>
          <span class="text-xs font-bold tabular-nums w-9 text-right" :style="{ color: barColor(memPercent) }">
            {{ memPercent.toFixed(0) }}%
          </span>
        </div>
        <!-- DISK -->
        <div class="flex items-center gap-2 min-w-[120px]">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest w-8">DISK</span>
          <div class="flex-1 h-1.5 rounded-full bg-slate-800 overflow-hidden">
            <div
              class="h-full rounded-full transition-all duration-500"
              :style="{ width: diskPercent + '%', backgroundColor: barColor(diskPercent) }"
            />
          </div>
          <span class="text-xs font-bold tabular-nums w-9 text-right" :style="{ color: barColor(diskPercent) }">
            {{ diskPercent.toFixed(0) }}%
          </span>
        </div>
      </div>
    </div>

    <!-- Right: runtime badge + bell -->
    <div class="flex items-center gap-4">
      <!-- Runtime indicator -->
      <div class="flex items-center gap-2 text-xs">
        <span
          class="inline-block h-2 w-2 rounded-full"
          :style="{ backgroundColor: containers.runtimeConnected ? '#10b981' : '#f43f5e' }"
        />
        <span class="font-medium text-slate-400">{{ containers.runtimeLabel }}</span>
      </div>

      <button class="p-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition-all relative">
        <Bell :size="18" />
        <span
          v-if="alertsStore.newAlertCount > 0"
          class="absolute top-1 right-1 w-2 h-2 bg-rose-500 rounded-full border-2 border-[#151923]"
        />
      </button>
    </div>
  </header>

  <!-- Runtime disconnection banner -->
  <div
    v-if="!containers.runtimeConnected"
    class="flex items-center gap-3 px-6 py-2 bg-amber-500/10 border-b border-amber-500/30"
  >
    <AlertTriangle :size="16" class="text-amber-500 shrink-0" />
    <span class="text-sm text-amber-400">
      {{ containers.runtimeLabel }} runtime disconnected — monitoring paused until connection is restored.
    </span>
  </div>
</template>
