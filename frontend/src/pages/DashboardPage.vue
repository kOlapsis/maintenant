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
import { inject, onMounted, onUnmounted, computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardStore, type UnifiedMonitor } from '@/stores/dashboard'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { useResourcesStore } from '@/stores/resources'
import { usePostureStore } from '@/stores/posture'
import SparklineChart from '@/components/ui/SparklineChart.vue'
import UpdateSummaryStrip from '@/components/dashboard/UpdateSummaryStrip.vue'
import IncidentFeedCard from '@/components/dashboard/IncidentFeedCard.vue'
import HostResourcesCard from '@/components/dashboard/HostResourcesCard.vue'
import UpdateBadge from '@/components/UpdateBadge.vue'
import RuntimeDegradedBanner from '@/components/RuntimeDegradedBanner.vue'
import { useContainersStore } from '@/stores/containers'
import { useUpdatesStore } from '@/stores/updates'
import { useEdition } from '@/composables/useEdition'
import {
  Zap,
  Cpu,
  Shield,
  ShieldCheck,
  Server,
  Cloud,
  ChevronRight,
  Activity,
  Filter,
  MoreVertical,
  Clock,
} from 'lucide-vue-next'

const { hasFeature } = useEdition()

const router = useRouter()
const detailSlideOver = inject(detailSlideOverKey)!
const dashboard = useDashboardStore()
const resources = useResourcesStore()
const updatesStore = useUpdatesStore()
const postureStore = usePostureStore()
const containersStore = useContainersStore()

const showPosture = computed(() => hasFeature('security_posture') && postureStore.posture !== null)

const filterOpen = ref(false)
const filterSearch = ref('')
const filterType = ref<string>('')
const filterIncidents = ref(false)

const SLIDEOVER_TYPES = new Set(['container', 'heartbeat', 'certificate'])

function selectService(monitor: UnifiedMonitor) {
  const entityId = monitor.id.split(':')[1] ?? ''
  if (SLIDEOVER_TYPES.has(monitor.type) && entityId) {
    detailSlideOver.openDetail(monitor.type as EntityType, entityId)
  } else {
    router.push(monitor.link)
  }
}

function clearFilters() {
  filterSearch.value = ''
  filterType.value = ''
  filterIncidents.value = false
}

const hasActiveFilters = computed(() =>
  filterSearch.value !== '' || filterType.value !== '' || filterIncidents.value,
)

const stats = computed(() => dashboard.globalStats)

// Filtered services
const filteredServices = computed(() => {
  let list = dashboard.monitors

  // Global search bar (header)
  const q = dashboard.searchQuery.toLowerCase().trim()
  if (q) {
    list = list.filter(
      (m) =>
        m.name.toLowerCase().includes(q) ||
        m.subtitle.toLowerCase().includes(q) ||
        m.statusLabel.toLowerCase().includes(q),
    )
  }

  // Local text search
  const local = filterSearch.value.toLowerCase().trim()
  if (local) {
    list = list.filter(
      (m) =>
        m.name.toLowerCase().includes(local) ||
        m.subtitle.toLowerCase().includes(local),
    )
  }

  // Type filter
  if (filterType.value) {
    list = list.filter((m) => m.type === filterType.value)
  }

  // Incidents only
  if (filterIncidents.value) {
    list = list.filter((m) => m.status === 'down' || m.status === 'warning')
  }

  return list
})

// Status dot style with pulse animation
function statusDotClass(status: string): string {
  if (status === 'ok') return 'bg-emerald-500 shadow-[0_0_8px_rgba(62,207,142,0.5)] animate-pulse'
  if (status === 'down') return 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.5)] animate-pulse'
  if (status === 'warning') return 'bg-amber-500 animate-pulse'
  return 'bg-slate-500'
}

// Type badge label
const typeLabels: Record<string, string> = {
  container: 'Container',
  endpoint: 'HTTP',
  heartbeat: 'Heartbeat',
  certificate: 'SSL',
  workload: 'Workload',
}

// Resource gauges
const totalCpu = computed(() =>
  resources.summary?.total_cpu_percent ?? 0,
)

function containerUpdateForMonitor(monitor: UnifiedMonitor) {
  if (monitor.type !== 'container') return null
  return updatesStore.updates.find(u => u.container_name === monitor.name) ?? null
}

// Summary cards
const summaryCards = computed(() => {
  const uptimePct = dashboard.monitors.length > 0
    ? ((stats.value.running / dashboard.monitors.length) * 100).toFixed(2)
    : '—'

  const avgLatency = (() => {
    const endpoints = dashboard.monitors.filter((m) => m.type === 'endpoint' && m.metricValue)
    if (!endpoints.length) return null
    const vals = endpoints.map((e) => parseFloat(e.metricValue ?? '0')).filter(Boolean)
    if (!vals.length) return null
    return Math.round(vals.reduce((s, v) => s + v, 0) / vals.length)
  })()

  const cpuVal = Math.round(totalCpu.value)
  const certOk = dashboard.certificateSummary.ok
  const certExpiring = (dashboard.certificateSummary as Record<string, number>).expiring ?? 0

  return [
    {
      title: 'Global Uptime',
      value: uptimePct !== '—' ? `${uptimePct}%` : '—',
      subtitle: `${stats.value.running} / ${dashboard.monitors.length} monitors`,
      trend: null,
      trendUp: null,
      icon: Activity,
      iconColor: 'text-pb-green-500',
      valueColor: 'text-pb-primary',
    },
    {
      title: 'Response Time',
      value: avgLatency ? `${avgLatency}ms` : 'N/A',
      subtitle: avgLatency ? 'Avg. endpoints' : 'No endpoints',
      trend: null,
      trendUp: null,
      icon: Zap,
      iconColor: 'text-amber-500',
      valueColor: 'text-pb-primary',
    },
    {
      title: 'Host Resources',
      value: `${cpuVal}%`,
      subtitle: 'CPU Usage',
      trend: null,
      trendUp: null,
      icon: Cpu,
      iconColor: 'text-pb-status-ok',
      valueColor: cpuVal > 80 ? 'text-pb-status-down' : cpuVal > 60 ? 'text-amber-400' : 'text-pb-primary',
    },
    {
      title: 'SSL Certificates',
      value: `${certOk} OK`,
      subtitle: certExpiring > 0 ? `${certExpiring} expiring soon` : 'All valid',
      trend: null,
      trendUp: null,
      icon: Shield,
      iconColor: 'text-pb-green-400',
      valueColor: certExpiring > 0 ? 'text-pb-status-down' : 'text-pb-primary',
    },
  ]
})

onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  updatesStore.fetchAllUpdates()
  // resources.fetchSummary + refresh timer live in HostResourcesCard
  if (hasFeature('security_posture')) postureStore.fetchPosture()
})

onUnmounted(() => {
  dashboard.disconnectAllSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
      <div class="max-w-7xl mx-auto space-y-4 sm:space-y-6 pb-12">

        <!-- Degraded mode banner (container monitoring unavailable) -->
        <RuntimeDegradedBanner v-if="!containersStore.isContainerMonitoringAvailable" />

        <!-- Summary Cards -->
        <div class="grid grid-cols-2 gap-2.5 sm:gap-5" :class="showPosture ? 'lg:grid-cols-5' : 'lg:grid-cols-4'">
          <div
            v-for="card in summaryCards"
            :key="card.title"
            class="bg-pb-surface p-3 sm:p-5 rounded-xl sm:rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg group cursor-default"
          >
            <div class="flex justify-between items-start mb-2 sm:mb-4">
              <div class="p-1.5 sm:p-2.5 bg-pb-primary rounded-lg sm:rounded-xl group-hover:scale-105 transition-transform">
                <component :is="card.icon" :size="14" class="sm:!w-[18px] sm:!h-[18px]" :class="card.iconColor" />
              </div>
              <span
                v-if="card.trend"
                :class="[
                  'text-[10px] font-bold px-1.5 py-0.5 rounded',
                  card.trendUp
                    ? 'bg-pb-status-ok text-pb-status-ok'
                    : 'bg-pb-status-down text-pb-status-down',
                ]"
              >{{ card.trend }}</span>
            </div>
            <p class="text-[9px] sm:text-[10px] text-slate-500 font-bold uppercase tracking-widest">{{ card.title }}</p>
            <h4 :class="['text-lg sm:text-2xl font-black mt-0.5', card.valueColor]">{{ card.value }}</h4>
            <p class="text-[9px] sm:text-[10px] text-slate-600 font-bold uppercase tracking-tight mt-0.5 truncate">{{ card.subtitle }}</p>
          </div>

          <!-- Security Posture card (Pro only) -->
          <RouterLink
            v-if="showPosture"
            to="/security"
            class="bg-pb-surface p-3 sm:p-5 rounded-xl sm:rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg group cursor-pointer"
          >
            <div class="flex justify-between items-start mb-2 sm:mb-4">
              <div class="p-1.5 sm:p-2.5 bg-pb-primary rounded-lg sm:rounded-xl group-hover:scale-105 transition-transform">
                <ShieldCheck :size="14" class="sm:!w-[18px] sm:!h-[18px]" :class="{
                  'text-pb-status-ok': postureStore.posture!.color === 'green',
                  'text-amber-500': postureStore.posture!.color === 'yellow',
                  'text-orange-500': postureStore.posture!.color === 'orange',
                  'text-red-500': postureStore.posture!.color === 'red',
                }" />
              </div>
            </div>
            <p class="text-[9px] sm:text-[10px] text-slate-500 font-bold uppercase tracking-widest">Security Posture</p>
            <h4 class="text-lg sm:text-2xl font-black mt-0.5" :class="{
              'text-pb-status-ok': postureStore.posture!.color === 'green',
              'text-amber-400': postureStore.posture!.color === 'yellow',
              'text-orange-400': postureStore.posture!.color === 'orange',
              'text-red-400': postureStore.posture!.color === 'red',
            }">{{ postureStore.posture!.score }}<span class="text-sm font-bold text-slate-600">/100</span></h4>
            <p class="text-[9px] sm:text-[10px] text-slate-600 font-bold uppercase tracking-tight mt-0.5 truncate">{{ postureStore.posture!.scored_count }} containers scored</p>
          </RouterLink>
        </div>

        <!-- Update Summary Strip -->
        <UpdateSummaryStrip />

        <!-- Monitor Table -->
        <div class="bg-pb-surface rounded-xl sm:rounded-2xl border border-slate-800 shadow-xl overflow-hidden">
          <!-- Table header -->
          <div class="px-4 sm:px-6 py-4 sm:py-5 border-b border-slate-800 flex flex-wrap justify-between items-center gap-3">
            <div>
              <h2 class="text-sm sm:text-base font-bold text-pb-primary">Unified Monitors</h2>
              <p class="text-[10px] sm:text-xs text-slate-500 mt-0.5">Containers, workloads, and external probes</p>
            </div>
            <div class="flex items-center gap-2">
              <button
                @click="filterOpen = !filterOpen"
                :class="[
                  'px-2.5 sm:px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 sm:gap-2 border',
                  hasActiveFilters
                    ? 'bg-pb-green-600/20 text-pb-green-400 border-pb-green-500/40 hover:bg-pb-green-600/30'
                    : 'bg-slate-800 hover:bg-slate-700 text-pb-primary border-slate-700',
                ]"
              >
                <Filter :size="13" />
                <span class="hidden sm:inline">Filter</span>
                <span v-if="hasActiveFilters" class="w-1.5 h-1.5 rounded-full bg-pb-green-400" />
              </button>
              <RouterLink
                to="/heartbeats"
                class="px-2.5 sm:px-3.5 py-1.5 bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 rounded-lg text-xs font-bold transition-all flex items-center gap-1.5 sm:gap-2 shadow-lg shadow-pb-green-500/20"
              >
                <Zap :size="13" class="fill-white" />
                <span class="hidden sm:inline">Add monitor</span>
                <span class="sm:hidden">Add</span>
              </RouterLink>
            </div>
          </div>

          <!-- Filter bar -->
          <div v-if="filterOpen" class="px-6 py-4 border-b border-slate-800 bg-pb-primary/40 flex flex-wrap items-center gap-3">
            <input
              v-model="filterSearch"
              type="text"
              placeholder="Search monitors..."
              class="px-3 py-1.5 bg-pb-primary border border-slate-700 rounded-lg text-xs text-pb-primary placeholder-slate-600 focus:outline-none focus:border-pb-green-500 w-full sm:w-52"
            />
            <select
              v-model="filterType"
              class="px-3 py-1.5 rounded-lg text-xs focus:outline-none focus:border-pb-green-500 appearance-none cursor-pointer"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
            >
              <option value="">All types</option>
              <option value="container">Container</option>
              <option value="workload">Workload</option>
              <option value="endpoint">HTTP</option>
              <option value="heartbeat">Heartbeat</option>
              <option value="certificate">SSL</option>
            </select>
            <button
              @click="filterIncidents = !filterIncidents"
              :class="[
                'px-3 py-1.5 rounded-lg text-xs font-medium transition-all border',
                filterIncidents
                  ? 'bg-pb-status-down text-pb-status-down border-rose-500/40'
                  : 'bg-pb-primary text-slate-400 border-slate-700 hover:border-slate-600',
              ]"
            >
              Incidents only
            </button>
            <button
              v-if="hasActiveFilters"
              @click="clearFilters"
              class="px-3 py-1.5 text-[10px] text-slate-500 hover:text-pb-secondary font-bold uppercase tracking-widest transition-colors"
            >
              Clear
            </button>
            <span class="ml-auto text-[10px] text-slate-600 font-bold">
              {{ filteredServices.length }} / {{ dashboard.monitors.length }} monitors
            </span>
          </div>

          <!-- Mobile card list -->
          <div class="md:hidden divide-y divide-slate-800/40">
            <div
              v-for="service in filteredServices"
              :key="'m-' + service.id"
              class="px-4 py-3 active:bg-slate-800/25 transition-colors cursor-pointer"
              @click="selectService(service)"
            >
              <div class="flex items-center gap-3">
                <div :class="['w-2.5 h-2.5 rounded-full shrink-0', statusDotClass(service.status)]" />
                <div class="min-w-0 flex-1">
                  <div class="flex items-center justify-between gap-2">
                    <p class="text-sm font-semibold text-pb-primary truncate">{{ service.name }}</p>
                    <span class="px-2 py-0.5 rounded bg-slate-800 text-slate-400 text-[9px] font-bold uppercase tracking-wider border border-slate-700/60 shrink-0">
                      {{ typeLabels[service.type] || service.type }}
                    </span>
                  </div>
                  <p class="text-[10px] text-slate-600 mt-0.5 flex items-center gap-1 truncate">
                    <Server v-if="service.type === 'container'" :size="9" />
                    <Cloud v-else-if="service.type === 'workload'" :size="9" />
                    <Clock v-else-if="service.type === 'heartbeat'" :size="9" />
                    <span>{{ service.subtitle }}</span>
                    <UpdateBadge v-if="service.type === 'container'" :update="containerUpdateForMonitor(service)" />
                  </p>
                </div>
                <ChevronRight :size="14" class="text-slate-700 shrink-0" />
              </div>
              <div v-if="service.metricValue" class="mt-2 ml-[22px] text-[10px] font-mono text-pb-green-400 font-bold">
                {{ service.metricValue }}
                <span class="text-slate-600 uppercase tracking-tighter ml-1">{{ service.metricLabel }}</span>
              </div>
            </div>
            <div v-if="filteredServices.length === 0" class="px-4 py-12 text-center">
              <Server :size="32" class="mx-auto text-slate-800 mb-3" />
              <p class="text-sm text-slate-600 font-medium">
                <template v-if="dashboard.searchQuery || hasActiveFilters">No monitors matching filters</template>
                <template v-else>No monitors yet. Start containers, deploy workloads, or add endpoints.</template>
              </p>
            </div>
          </div>

          <!-- Desktop table -->
          <div class="hidden md:block overflow-x-auto">
            <table class="w-full text-left border-collapse">
              <thead>
                <tr class="bg-pb-primary/60 text-slate-500 text-[10px] uppercase tracking-widest font-bold border-b border-slate-800/60">
                  <th class="px-6 py-3.5">Status / Name</th>
                  <th class="px-6 py-3.5">Type</th>
                  <th class="px-6 py-3.5">Details / Resources</th>
                  <th class="px-6 py-3.5">History (90d)</th>
                  <th class="px-6 py-3.5 text-right">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-800/40">
                <tr
                  v-for="service in filteredServices"
                  :key="service.id"
                  class="group hover:bg-slate-800/25 transition-all cursor-pointer"
                  @click="selectService(service)"
                >
                  <!-- Name / Status -->
                  <td class="px-6 py-4">
                    <div class="flex items-center gap-4">
                      <div :class="['w-2.5 h-2.5 rounded-full shrink-0', statusDotClass(service.status)]" />
                      <div class="min-w-0">
                        <p class="text-sm font-semibold text-pb-primary group-hover:text-pb-green-400 transition-colors truncate">
                          {{ service.name }}
                        </p>
                        <p class="text-[10px] text-slate-600 mt-0.5 flex items-center gap-1 truncate">
                          <Server v-if="service.type === 'container'" :size="9" />
                          <Cloud v-else-if="service.type === 'workload'" :size="9" />
                          <Clock v-else-if="service.type === 'heartbeat'" :size="9" />
                          <span>{{ service.subtitle }}</span>
                          <UpdateBadge v-if="service.type === 'container'" :update="containerUpdateForMonitor(service)" />
                        </p>
                      </div>
                    </div>
                  </td>

                  <!-- Type badge -->
                  <td class="px-6 py-4">
                    <span class="px-2 py-0.5 rounded bg-slate-800 text-slate-400 text-[9px] font-bold uppercase tracking-wider border border-slate-700/60">
                      {{ typeLabels[service.type] || service.type }}
                    </span>
                  </td>

                  <!-- Resources / sparkline -->
                  <td class="px-6 py-4">
                    <div v-if="service.sparklineData && service.sparklineData.length > 1" class="flex items-center gap-3">
                      <SparklineChart
                        :data="service.sparklineData"
                        :width="52"
                        :height="24"
                        :color="service.status === 'down' ? '#475569' : '#3b82f6'"
                      />
                      <div class="text-[9px] space-y-0.5">
                        <p class="text-pb-primary font-mono font-bold">{{ service.metricValue }}</p>
                        <p class="text-slate-600 uppercase tracking-tighter">{{ service.metricLabel }}</p>
                      </div>
                    </div>
                    <div v-else-if="service.metricValue" class="text-[10px] font-mono text-pb-green-400 font-bold">
                      {{ service.metricValue }}
                      <p class="text-[9px] text-slate-600 uppercase tracking-tighter mt-0.5">{{ service.metricLabel }}</p>
                    </div>
                    <span v-else class="text-[10px] text-slate-700 font-medium">N/A</span>
                  </td>

                  <!-- 90-day history bars -->
                  <td class="px-6 py-4">
                    <div class="flex gap-[2px] items-center h-5">
                      <div v-if="service.status === 'ok'" class="flex gap-[2px]">
                        <div v-for="i in 30" :key="i" class="h-4 w-[3px] rounded-full bg-emerald-500/35 hover:bg-emerald-500/70 transition-colors cursor-help" />
                      </div>
                      <div v-else-if="service.status === 'down'" class="flex gap-[2px]">
                        <div v-for="i in 27" :key="i" class="h-4 w-[3px] rounded-full bg-emerald-500/35" />
                        <div class="h-4 w-[3px] rounded-full bg-rose-500" />
                        <div class="h-4 w-[3px] rounded-full bg-rose-500" />
                        <div class="h-4 w-[3px] rounded-full bg-rose-500" />
                      </div>
                      <div v-else class="flex gap-[2px]">
                        <div v-for="i in 28" :key="i" class="h-4 w-[3px] rounded-full bg-emerald-500/35" />
                        <div class="h-4 w-[3px] rounded-full bg-amber-400" />
                        <div class="h-4 w-[3px] rounded-full bg-emerald-500/35" />
                      </div>
                    </div>
                    <div class="flex justify-between mt-1.5 text-[9px] text-slate-700 font-bold uppercase tracking-tight">
                      <span>90d</span>
                      <span>Today</span>
                    </div>
                  </td>

                  <!-- Actions -->
                  <td class="px-6 py-4 text-right">
                    <button
                      :aria-label="`Open details for ${service.name}`"
                      class="p-1.5 text-slate-600 hover:text-pb-secondary hover:bg-slate-700/60 rounded-lg transition-all"
                      @click.stop="selectService(service)"
                    >
                      <MoreVertical :size="16" />
                    </button>
                  </td>
                </tr>

                <!-- Empty state -->
                <tr v-if="filteredServices.length === 0">
                  <td colspan="5" class="px-6 py-16 text-center">
                    <Server :size="32" class="mx-auto text-slate-800 mb-3" />
                    <p class="text-sm text-slate-600 font-medium">
                      <template v-if="dashboard.searchQuery || hasActiveFilters">No monitors matching filters</template>
                      <template v-else>No monitors yet. Start containers, deploy workloads, or add endpoints.</template>
                    </p>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Bottom Grid -->
        <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 sm:gap-5">
          <IncidentFeedCard class="lg:col-span-2" />
          <HostResourcesCard />
        </div>
      </div>

  </div>
</template>
