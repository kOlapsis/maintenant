<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardStore, type UnifiedMonitor } from '@/stores/dashboard'
import { useResourcesStore } from '@/stores/resources'
import { useAlertsStore } from '@/stores/alerts'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { usePostureStore } from '@/stores/posture'
import type { Alert } from '@/services/alertApi'
import SparklineChart from '@/components/ui/SparklineChart.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import UpdateSummaryStrip from '@/components/dashboard/UpdateSummaryStrip.vue'
import UpdateBadge from '@/components/UpdateBadge.vue'
import { useUpdatesStore } from '@/stores/updates'
import { useEdition } from '@/composables/useEdition'
import { timeAgo } from '@/utils/time'
import {
  Zap,
  Cpu,
  Shield,
  ShieldCheck,
  Server,
  ChevronRight,
  Activity,
  Filter,
  MoreVertical,
  Clock,
} from 'lucide-vue-next'

const { hasFeature } = useEdition()

const router = useRouter()
const dashboard = useDashboardStore()
const resources = useResourcesStore()
const alertsStore = useAlertsStore()
const statusAdmin = useStatusAdminStore()
const updatesStore = useUpdatesStore()
const postureStore = usePostureStore()

const showPosture = computed(() => hasFeature('security_posture') && postureStore.posture !== null)

const selectedService = ref<UnifiedMonitor | null>(null)
const slideOpen = ref(false)
const filterOpen = ref(false)
const filterSearch = ref('')
const filterType = ref<string>('')
const filterIncidents = ref(false)

function selectService(monitor: UnifiedMonitor) {
  selectedService.value = monitor
  slideOpen.value = true
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
}

// Resource gauges
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

function gaugeBarColor(val: number, thresholds = { warn: 60, crit: 80 }): string {
  if (val > thresholds.crit) return 'bg-rose-500'
  if (val > thresholds.warn) return 'bg-amber-500'
  return 'bg-pb-green-500'
}

// Unified incident feed: active alerts + status page incidents
const incidentFeed = computed(() => {
  const items: {
    id: string
    service: string
    message: string
    time: string
    color: string
    icon: string
    route: string
  }[] = []

  // Collect all active alerts (deduplicated, sorted by fired_at desc)
  const allActive: Alert[] = [
    ...(alertsStore.activeAlerts.critical ?? []),
    ...(alertsStore.activeAlerts.warning ?? []),
    ...(alertsStore.activeAlerts.info ?? []),
  ].sort((a, b) => new Date(b.fired_at || b.created_at).getTime() - new Date(a.fired_at || a.created_at).getTime())

  for (const alert of allActive.slice(0, 6)) {
    const color =
      alert.severity === 'critical' ? 'bg-rose-500' :
      alert.severity === 'warning'  ? 'bg-amber-500' :
      'bg-pb-green-500'
    const route = alertEntityRoute(alert)
    items.push({
      id: `alert-${alert.id}`,
      service: alert.entity_name || alert.source || `Alert #${alert.id}`,
      message: alert.message,
      time: formatRelativeTime(alert.fired_at || alert.created_at),
      color,
      icon: 'alert',
      route,
    })
  }

  // Active status page incidents (non-resolved)
  for (const inc of statusAdmin.incidents.filter((i) => i.status !== 'resolved').slice(0, 3)) {
    const color =
      inc.severity === 'critical' ? 'bg-rose-500' :
      inc.severity === 'major'    ? 'bg-rose-500' :
      inc.severity === 'minor'    ? 'bg-amber-500' :
      'bg-pb-green-400'
    items.push({
      id: `inc-${inc.id}`,
      service: inc.title,
      message: inc.updates?.[0]?.message ?? `Incident ${inc.status}`,
      time: formatRelativeTime(inc.created_at),
      color,
      icon: 'status',
      route: '/status-admin',
    })
  }

  return items.slice(0, 6)
})

function alertEntityRoute(alert: Alert): string {
  switch (alert.entity_type) {
    case 'container': return '/containers'
    case 'endpoint': return '/endpoints'
    case 'heartbeat': return '/heartbeats'
    case 'certificate': return '/certificates'
    default: return '/alerts'
  }
}

function navigateToIncident(inc: { route: string }) {
  router.push(inc.route)
}

const formatRelativeTime = timeAgo

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
      valueColor: 'text-white',
    },
    {
      title: 'Response Time',
      value: avgLatency ? `${avgLatency}ms` : 'N/A',
      subtitle: avgLatency ? 'Avg. endpoints' : 'No endpoints',
      trend: null,
      trendUp: null,
      icon: Zap,
      iconColor: 'text-amber-500',
      valueColor: 'text-white',
    },
    {
      title: 'Host Resources',
      value: `${cpuVal}%`,
      subtitle: 'CPU Usage',
      trend: null,
      trendUp: null,
      icon: Cpu,
      iconColor: 'text-emerald-500',
      valueColor: cpuVal > 80 ? 'text-rose-400' : cpuVal > 60 ? 'text-amber-400' : 'text-white',
    },
    {
      title: 'SSL Certificates',
      value: `${certOk} OK`,
      subtitle: certExpiring > 0 ? `${certExpiring} expiring soon` : 'All valid',
      trend: null,
      trendUp: null,
      icon: Shield,
      iconColor: 'text-pb-green-400',
      valueColor: certExpiring > 0 ? 'text-rose-400' : 'text-white',
    },
  ]
})

let summaryTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  alertsStore.fetchAlerts()
  alertsStore.fetchActiveAlerts()
  updatesStore.fetchAllUpdates()
  resources.fetchSummary()
  if (hasFeature('incidents')) statusAdmin.fetchIncidents()
  if (hasFeature('security_posture')) postureStore.fetchPosture()

  summaryTimer = setInterval(() => resources.fetchSummary(), 3_000)
})

onUnmounted(() => {
  dashboard.disconnectAllSSE()
  if (summaryTimer) clearInterval(summaryTimer)
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
      <div class="max-w-7xl mx-auto space-y-4 sm:space-y-6 pb-12">

        <!-- Summary Cards -->
        <div class="grid grid-cols-2 gap-2.5 sm:gap-5" :class="showPosture ? 'lg:grid-cols-5' : 'lg:grid-cols-4'">
          <div
            v-for="card in summaryCards"
            :key="card.title"
            class="bg-[#12151C] p-3 sm:p-5 rounded-xl sm:rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg group cursor-default"
          >
            <div class="flex justify-between items-start mb-2 sm:mb-4">
              <div class="p-1.5 sm:p-2.5 bg-[#0B0E13] rounded-lg sm:rounded-xl group-hover:scale-105 transition-transform">
                <component :is="card.icon" :size="14" class="sm:!w-[18px] sm:!h-[18px]" :class="card.iconColor" />
              </div>
              <span
                v-if="card.trend"
                :class="[
                  'text-[10px] font-bold px-1.5 py-0.5 rounded',
                  card.trendUp
                    ? 'bg-emerald-500/10 text-emerald-500'
                    : 'bg-rose-500/10 text-rose-500',
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
            class="bg-[#12151C] p-3 sm:p-5 rounded-xl sm:rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg group cursor-pointer"
          >
            <div class="flex justify-between items-start mb-2 sm:mb-4">
              <div class="p-1.5 sm:p-2.5 bg-[#0B0E13] rounded-lg sm:rounded-xl group-hover:scale-105 transition-transform">
                <ShieldCheck :size="14" class="sm:!w-[18px] sm:!h-[18px]" :class="{
                  'text-emerald-500': postureStore.posture!.color === 'green',
                  'text-amber-500': postureStore.posture!.color === 'yellow',
                  'text-orange-500': postureStore.posture!.color === 'orange',
                  'text-red-500': postureStore.posture!.color === 'red',
                }" />
              </div>
            </div>
            <p class="text-[9px] sm:text-[10px] text-slate-500 font-bold uppercase tracking-widest">Security Posture</p>
            <h4 class="text-lg sm:text-2xl font-black mt-0.5" :class="{
              'text-emerald-400': postureStore.posture!.color === 'green',
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
        <div class="bg-[#12151C] rounded-xl sm:rounded-2xl border border-slate-800 shadow-xl overflow-hidden">
          <!-- Table header -->
          <div class="px-4 sm:px-6 py-4 sm:py-5 border-b border-slate-800 flex flex-wrap justify-between items-center gap-3">
            <div>
              <h2 class="text-sm sm:text-base font-bold text-white">Unified Monitors</h2>
              <p class="text-[10px] sm:text-xs text-slate-500 mt-0.5">Docker auto-discovery and external probes</p>
            </div>
            <div class="flex items-center gap-2">
              <button
                @click="filterOpen = !filterOpen"
                :class="[
                  'px-2.5 sm:px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 sm:gap-2 border',
                  hasActiveFilters
                    ? 'bg-pb-green-600/20 text-pb-green-400 border-pb-green-500/40 hover:bg-pb-green-600/30'
                    : 'bg-slate-800 hover:bg-slate-700 text-slate-200 border-slate-700',
                ]"
              >
                <Filter :size="13" />
                <span class="hidden sm:inline">Filter</span>
                <span v-if="hasActiveFilters" class="w-1.5 h-1.5 rounded-full bg-pb-green-400" />
              </button>
              <RouterLink
                to="/heartbeats"
                class="px-2.5 sm:px-3.5 py-1.5 bg-pb-green-600 hover:bg-pb-green-500 text-white rounded-lg text-xs font-bold transition-all flex items-center gap-1.5 sm:gap-2 shadow-lg shadow-pb-green-500/20"
              >
                <Zap :size="13" class="fill-white" />
                <span class="hidden sm:inline">Add monitor</span>
                <span class="sm:hidden">Add</span>
              </RouterLink>
            </div>
          </div>

          <!-- Filter bar -->
          <div v-if="filterOpen" class="px-6 py-4 border-b border-slate-800 bg-[#0B0E13]/40 flex flex-wrap items-center gap-3">
            <input
              v-model="filterSearch"
              type="text"
              placeholder="Search monitors..."
              class="px-3 py-1.5 bg-[#0B0E13] border border-slate-700 rounded-lg text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-pb-green-500 w-full sm:w-52"
            />
            <select
              v-model="filterType"
              class="px-3 py-1.5 bg-[#0B0E13] border border-slate-700 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-pb-green-500 appearance-none cursor-pointer"
            >
              <option value="">All types</option>
              <option value="container">Container</option>
              <option value="endpoint">HTTP</option>
              <option value="heartbeat">Heartbeat</option>
              <option value="certificate">SSL</option>
            </select>
            <button
              @click="filterIncidents = !filterIncidents"
              :class="[
                'px-3 py-1.5 rounded-lg text-xs font-medium transition-all border',
                filterIncidents
                  ? 'bg-rose-500/15 text-rose-400 border-rose-500/40'
                  : 'bg-[#0B0E13] text-slate-400 border-slate-700 hover:border-slate-600',
              ]"
            >
              Incidents only
            </button>
            <button
              v-if="hasActiveFilters"
              @click="clearFilters"
              class="px-3 py-1.5 text-[10px] text-slate-500 hover:text-slate-300 font-bold uppercase tracking-widest transition-colors"
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
                    <p class="text-sm font-semibold text-slate-100 truncate">{{ service.name }}</p>
                    <span class="px-2 py-0.5 rounded bg-slate-800 text-slate-400 text-[9px] font-bold uppercase tracking-wider border border-slate-700/60 shrink-0">
                      {{ typeLabels[service.type] || service.type }}
                    </span>
                  </div>
                  <p class="text-[10px] text-slate-600 mt-0.5 flex items-center gap-1 truncate">
                    <Server v-if="service.type === 'container'" :size="9" />
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
                <template v-else>No monitors. Start Docker containers or add endpoints.</template>
              </p>
            </div>
          </div>

          <!-- Desktop table -->
          <div class="hidden md:block overflow-x-auto">
            <table class="w-full text-left border-collapse">
              <thead>
                <tr class="bg-[#0B0E13]/60 text-slate-500 text-[10px] uppercase tracking-widest font-bold border-b border-slate-800/60">
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
                        <p class="text-sm font-semibold text-slate-100 group-hover:text-pb-green-400 transition-colors truncate">
                          {{ service.name }}
                        </p>
                        <p class="text-[10px] text-slate-600 mt-0.5 flex items-center gap-1 truncate">
                          <Server v-if="service.type === 'container'" :size="9" />
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
                        <p class="text-slate-200 font-mono font-bold">{{ service.metricValue }}</p>
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
                      class="p-1.5 text-slate-600 hover:text-slate-300 hover:bg-slate-700/60 rounded-lg transition-all"
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
                      <template v-else>No monitors. Start Docker containers or add endpoints.</template>
                    </p>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Bottom Grid -->
        <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 sm:gap-5">

          <!-- Incident Activity Feed -->
          <div class="lg:col-span-2 bg-[#12151C] rounded-xl sm:rounded-2xl border border-slate-800 p-4 sm:p-6">
            <div class="flex justify-between items-center mb-5">
              <h3 class="text-sm font-bold text-white flex items-center gap-2.5">
                <Activity :size="15" class="text-pb-green-500" />
                Incident Activity Feed
              </h3>
              <RouterLink
                to="/alerts"
                class="text-[10px] text-pb-green-500 hover:text-pb-green-400 font-bold uppercase tracking-widest transition-colors"
              >
                View full history
              </RouterLink>
            </div>

            <div v-if="incidentFeed.length > 0" class="space-y-1">
              <div
                v-for="(inc, idx) in incidentFeed"
                :key="inc.id"
                class="flex gap-4 p-3 rounded-xl hover:bg-slate-800/40 transition-all border border-transparent hover:border-slate-800/60 group cursor-pointer"
                @click="navigateToIncident(inc)"
              >
                <div class="flex flex-col items-center gap-1 shrink-0">
                  <div :class="['w-2 h-2 rounded-full mt-1.5 shrink-0', inc.color]" />
                  <div v-if="idx < incidentFeed.length - 1" class="w-px flex-1 bg-slate-800" />
                </div>
                <div class="flex-1 min-w-0">
                  <div class="flex justify-between items-center mb-0.5">
                    <span class="text-xs font-semibold text-slate-200 group-hover:text-pb-green-400 transition-colors tracking-tight truncate mr-3">{{ inc.service }}</span>
                    <span class="text-[10px] text-slate-600 font-bold shrink-0">{{ inc.time }}</span>
                  </div>
                  <p class="text-[11px] text-slate-500 truncate">{{ inc.message }}</p>
                </div>
                <ChevronRight :size="13" class="text-slate-700 group-hover:text-slate-400 self-center shrink-0 transition-colors" />
              </div>
            </div>

            <div v-else class="flex flex-col items-center justify-center py-10 gap-3">
              <div class="w-10 h-10 rounded-full bg-emerald-500/10 flex items-center justify-center">
                <Activity :size="18" class="text-emerald-500" />
              </div>
              <p class="text-sm text-slate-600 font-medium">No recent incidents</p>
              <p class="text-[10px] text-slate-700">All services operating normally</p>
            </div>
          </div>

          <!-- Host Resources -->
          <div class="bg-[#12151C] rounded-xl sm:rounded-2xl border border-slate-800 p-4 sm:p-6">
            <div class="flex items-center gap-2.5 mb-5">
              <Server :size="15" class="text-emerald-500" />
              <h3 class="text-sm font-bold text-white">Host Resources</h3>
            </div>

            <div class="space-y-5">
              <!-- CPU -->
              <div class="space-y-1.5">
                <div class="flex justify-between items-center text-[10px] font-bold uppercase tracking-widest">
                  <span class="text-slate-500">CPU Usage</span>
                  <span class="text-slate-200">{{ Math.round(totalCpu) }}%</span>
                </div>
                <div class="h-1.5 w-full bg-[#0B0E13] rounded-full border border-slate-800 overflow-hidden">
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
                  <span class="text-slate-500">RAM Memory</span>
                  <span class="text-slate-200 text-right">
                    {{ resources.formatBytes(totalMemUsed) }} / {{ resources.formatBytes(totalMemLimit) }}
                  </span>
                </div>
                <div class="h-1.5 w-full bg-[#0B0E13] rounded-full border border-slate-800 overflow-hidden">
                  <div
                    class="h-full rounded-full transition-all duration-700"
                    :class="gaugeBarColor(memPercent, { warn: 70, crit: 85 })"
                    :style="{ width: `${Math.min(memPercent, 100)}%` }"
                  />
                </div>
              </div>

              <!-- Stats -->
              <div class="pt-4 border-t border-slate-800 space-y-2.5">
                <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
                  <span class="text-slate-500">Containers</span>
                  <span class="text-slate-300 font-mono">{{ Object.keys(resources.snapshots).length }} active</span>
                </div>
                <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
                  <span class="text-slate-500">Monitors</span>
                  <span class="text-slate-300 font-mono">{{ dashboard.monitors.length }} total</span>
                </div>
                <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
                  <span class="text-slate-500">Availability</span>
                  <span class="text-emerald-400 font-mono">
                    {{ dashboard.monitors.length > 0 ? ((stats.running / dashboard.monitors.length) * 100).toFixed(1) : '—' }}%
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

    <!-- Slide-over detail panel -->
    <SlideOverPanel v-model:open="slideOpen" :title="selectedService?.name || ''">
      <template v-if="selectedService">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-6">
          <div class="bg-[#0B0E13] p-4 rounded-xl border border-slate-800">
            <p class="text-[10px] text-slate-500 font-bold uppercase mb-1.5 tracking-widest">Status</p>
            <div class="flex items-center gap-2">
              <div :class="['w-2.5 h-2.5 rounded-full', statusDotClass(selectedService.status)]" />
              <p class="text-white font-semibold text-sm">{{ selectedService.statusLabel }}</p>
            </div>
          </div>
          <div v-if="selectedService.metricValue" class="bg-[#0B0E13] p-4 rounded-xl border border-slate-800">
            <p class="text-[10px] text-slate-500 font-bold uppercase mb-1.5 tracking-widest">{{ selectedService.metricLabel || 'Metric' }}</p>
            <p class="text-white font-semibold text-sm font-mono">{{ selectedService.metricValue }}</p>
          </div>
        </div>

        <div class="space-y-3 mb-6">
          <div>
            <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-2">Details</h4>
            <div class="bg-[#0B0E13] rounded-xl p-4 border border-slate-800 space-y-2.5">
              <div class="flex justify-between text-xs">
                <span class="text-slate-500">Type</span>
                <span class="text-slate-300 capitalize">{{ selectedService.type }}</span>
              </div>
              <div class="flex justify-between text-xs">
                <span class="text-slate-500">Source</span>
                <span class="text-slate-300 truncate ml-4 text-right">{{ selectedService.subtitle }}</span>
              </div>
              <div v-if="selectedService.group" class="flex justify-between text-xs">
                <span class="text-slate-500">Group</span>
                <span class="text-slate-300">{{ selectedService.group }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-if="selectedService.sparklineData && selectedService.sparklineData.length > 1" class="mb-6">
          <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-2">Trend</h4>
          <div class="bg-[#0B0E13] rounded-xl p-4 border border-slate-800">
            <SparklineChart
              :data="selectedService.sparklineData"
              :width="320"
              :height="64"
              color="#3b82f6"
              :fill-opacity="0.12"
            />
          </div>
        </div>

        <div class="pt-5 border-t border-slate-800 flex gap-3">
          <RouterLink
            :to="selectedService.link"
            class="flex-1 py-2.5 bg-pb-green-600 hover:bg-pb-green-500 text-white rounded-xl text-xs font-bold transition-all shadow-lg text-center"
            @click="slideOpen = false"
          >
            View details
          </RouterLink>
        </div>
      </template>
    </SlideOverPanel>
  </div>
</template>
