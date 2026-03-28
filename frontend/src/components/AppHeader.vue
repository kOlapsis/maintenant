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
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardStore } from '@/stores/dashboard'
import { useAlertsStore } from '@/stores/alerts'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import { Search, Bell, AlertTriangle, Box, Globe, Heart, ShieldCheck, Cpu } from 'lucide-vue-next'
import RuntimeBadge from '@/components/RuntimeBadge.vue'

const router = useRouter()
const dashboard = useDashboardStore()
const alertsStore = useAlertsStore()
const resources = useResourcesStore()
const containers = useContainersStore()

let summaryInterval: ReturnType<typeof setInterval> | null = null

// Global SSE connections + initial data fetch — always active while the app shell is mounted
onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  resources.fetchSummary()
  summaryInterval = setInterval(() => resources.fetchSummary(), 30_000)
})

onUnmounted(() => {
  dashboard.disconnectAllSSE()
  if (summaryInterval) clearInterval(summaryInterval)
})

const sourceRouteMap: Record<string, { route: string; label: string; icon: typeof Box }> = {
  container: { route: 'containers', label: 'Containers', icon: Box },
  endpoint: { route: 'endpoints', label: 'Endpoints', icon: Globe },
  heartbeat: { route: 'heartbeats', label: 'Heartbeats', icon: Heart },
  certificate: { route: 'certificates', label: 'Certificates', icon: ShieldCheck },
  resource: { route: 'containers', label: 'Resources', icon: Cpu },
}

const alertsBySource = computed(() => {
  const all = [
    ...alertsStore.activeAlerts.critical,
    ...alertsStore.activeAlerts.warning,
    ...alertsStore.activeAlerts.info,
  ]
  const grouped: Record<string, { count: number; critical: number; warning: number }> = {}
  for (const a of all) {
    if (!grouped[a.source]) grouped[a.source] = { count: 0, critical: 0, warning: 0 }
    grouped[a.source]!.count++
    if (a.severity === 'critical') grouped[a.source]!.critical++
    else if (a.severity === 'warning') grouped[a.source]!.warning++
  }
  return grouped
})

const sourceKeys = computed(() => Object.keys(alertsBySource.value))

const bellOpen = ref(false)
let closeTimeout: ReturnType<typeof setTimeout> | null = null

function onBellEnter() {
  if (closeTimeout) { clearTimeout(closeTimeout); closeTimeout = null }
  if (alertsStore.totalActiveCount > 0) {
    bellOpen.value = true
  }
}

function onBellLeave() {
  closeTimeout = setTimeout(() => { bellOpen.value = false }, 150)
}

function onBellClick() {
  if (alertsStore.totalActiveCount === 0) {
    router.push({ name: 'alerts' })
    return
  }
  if (sourceKeys.value.length === 1) {
    const source = sourceKeys.value[0]!
    const mapped = sourceRouteMap[source]
    router.push({ name: mapped?.route ?? 'alerts' })
    bellOpen.value = false
    return
  }
  bellOpen.value = !bellOpen.value
}

function navigateToSource(source: string) {
  const mapped = sourceRouteMap[source]
  router.push({ name: mapped?.route ?? 'alerts' })
  bellOpen.value = false
}

const totalCpu = computed(() => {
  return Math.min(resources.summary?.total_cpu_percent ?? 0, 100)
})

const memPercent = computed(() => {
  const used = resources.summary?.total_mem_used ?? 0
  const limit = resources.summary?.total_mem_limit ?? 0
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
  <header class="hidden md:flex h-16 shrink-0 border-b border-slate-800 items-center justify-between px-6 bg-[#12151C]/60 backdrop-blur-md z-10">
    <div class="flex items-center gap-5">
      <!-- Search -->
      <div class="relative group">
        <Search
          :size="15"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500 group-focus-within:text-pb-green-400 transition-colors"
        />
        <input
          v-model="dashboard.searchQuery"
          type="text"
          placeholder="Search services..."
          class="bg-[#0B0E13] border border-slate-800 rounded-lg py-2 pl-9 pr-4 text-sm w-72 focus:outline-none focus:ring-1 focus:ring-pb-green-500/60 focus:border-pb-green-500/40 transition-all text-slate-200 placeholder:text-slate-600"
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
      <!-- Runtime badge with popover -->
      <RuntimeBadge />

      <div
        class="relative"
        @mouseenter="onBellEnter"
        @mouseleave="onBellLeave"
      >
        <button
          @click="onBellClick"
          class="p-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition-all relative"
        >
          <Bell :size="18" />
          <span
            v-if="alertsStore.totalActiveCount > 0"
            class="absolute top-1.5 right-1.5 h-2 w-2 rounded-full"
            :class="alertsStore.activeAlerts.critical.length > 0 ? 'bg-rose-500' : 'bg-amber-500'"
          >
            <span
              class="absolute inset-0 rounded-full animate-ping"
              :class="alertsStore.activeAlerts.critical.length > 0 ? 'bg-rose-500' : 'bg-amber-500'"
            />
          </span>
        </button>

        <!-- Popover menu -->
        <Transition
          enter-active-class="transition duration-100 ease-out"
          enter-from-class="opacity-0 scale-95 -translate-y-1"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition duration-75 ease-in"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 -translate-y-1"
        >
          <div
            v-if="bellOpen"
            class="absolute right-0 top-full mt-2 w-56 rounded-xl border border-slate-700 bg-[#12151C] shadow-2xl shadow-black/40 overflow-hidden z-50"
            @mouseenter="onBellEnter"
            @mouseleave="onBellLeave"
          >
            <div class="px-3 py-2.5 border-b border-slate-800 flex items-center justify-between">
              <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Active alerts</span>
              <span
                class="min-w-[20px] h-5 flex items-center justify-center rounded-full text-[10px] font-bold px-1.5"
                :class="alertsStore.activeAlerts.critical.length > 0 ? 'bg-rose-500/15 text-rose-400' : 'bg-amber-500/15 text-amber-400'"
              >
                {{ alertsStore.totalActiveCount }}
              </span>
            </div>
            <div class="py-1">
              <button
                v-for="source in sourceKeys"
                :key="source"
                @click="navigateToSource(source)"
                class="w-full flex items-center gap-3 px-3 py-2 text-sm text-slate-300 hover:bg-slate-800/60 transition-colors"
              >
                <component
                  :is="sourceRouteMap[source]?.icon ?? AlertTriangle"
                  :size="14"
                  class="shrink-0"
                  :class="alertsBySource[source]?.critical ? 'text-rose-400' : alertsBySource[source]?.warning ? 'text-amber-400' : 'text-pb-green-400'"
                />
                <span class="flex-1 text-left">{{ sourceRouteMap[source]?.label ?? source }}</span>
                <span
                  class="min-w-[20px] h-5 flex items-center justify-center rounded-full text-[10px] font-bold px-1.5"
                  :class="alertsBySource[source]?.critical ? 'bg-rose-500/15 text-rose-400' : alertsBySource[source]?.warning ? 'bg-amber-500/15 text-amber-400' : 'bg-pb-green-500/15 text-pb-green-400'"
                >
                  {{ alertsBySource[source]?.count }}
                </span>
              </button>
            </div>
            <div class="border-t border-slate-800">
              <button
                @click="navigateToSource('_all')"
                class="w-full px-3 py-2 text-[11px] font-medium text-slate-500 hover:text-slate-300 hover:bg-slate-800/40 transition-colors text-center"
              >
                View all alerts
              </button>
            </div>
          </div>
        </Transition>
      </div>
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
