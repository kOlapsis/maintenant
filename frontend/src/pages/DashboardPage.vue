<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardStore, type UnifiedMonitor } from '@/stores/dashboard'
import { useResourcesStore } from '@/stores/resources'
import { useAlertsStore } from '@/stores/alerts'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import type { Alert } from '@/services/alertApi'
import SparklineChart from '@/components/ui/SparklineChart.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import UpdateSummaryStrip from '@/components/dashboard/UpdateSummaryStrip.vue'
import { useEdition } from '@/composables/useEdition'
import { timeAgoFr } from '@/utils/time'
import {
  Zap,
  Cpu,
  Shield,
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
  if (status === 'ok') return 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)] animate-pulse'
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
  Object.values(resources.snapshots).reduce((sum, s) => sum + s.cpu_percent, 0),
)
const totalMemUsed = computed(() =>
  Object.values(resources.snapshots).reduce((sum, s) => sum + s.mem_used, 0),
)
const totalMemLimit = computed(() =>
  Object.values(resources.snapshots).reduce((sum, s) => sum + s.mem_limit, 0),
)
const memPercent = computed(() => {
  if (totalMemLimit.value === 0) return 0
  return (totalMemUsed.value / totalMemLimit.value) * 100
})

function gaugeBarColor(val: number, thresholds = { warn: 60, crit: 80 }): string {
  if (val > thresholds.crit) return 'bg-rose-500'
  if (val > thresholds.warn) return 'bg-amber-500'
  return 'bg-blue-500'
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
      'bg-blue-500'
    const route = alertEntityRoute(alert)
    items.push({
      id: `alert-${alert.id}`,
      service: alert.entity_name || alert.source || `Alerte #${alert.id}`,
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
      'bg-blue-400'
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

const formatRelativeTime = timeAgoFr

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
      title: 'Uptime Global',
      value: uptimePct !== '—' ? `${uptimePct}%` : '—',
      subtitle: `${stats.value.running} / ${dashboard.monitors.length} moniteurs`,
      trend: null,
      trendUp: null,
      icon: Activity,
      iconColor: 'text-blue-500',
      valueColor: 'text-white',
    },
    {
      title: 'Temps de Réponse',
      value: avgLatency ? `${avgLatency}ms` : 'N/A',
      subtitle: avgLatency ? 'Moyenne endpoints' : 'Aucun endpoint',
      trend: null,
      trendUp: null,
      icon: Zap,
      iconColor: 'text-amber-500',
      valueColor: 'text-white',
    },
    {
      title: 'Ressources Hôte',
      value: `${cpuVal}%`,
      subtitle: 'CPU Usage',
      trend: null,
      trendUp: null,
      icon: Cpu,
      iconColor: 'text-emerald-500',
      valueColor: cpuVal > 80 ? 'text-rose-400' : cpuVal > 60 ? 'text-amber-400' : 'text-white',
    },
    {
      title: 'Certificats SSL',
      value: `${certOk} OK`,
      subtitle: certExpiring > 0 ? `${certExpiring} expire bientôt` : 'Tous valides',
      trend: null,
      trendUp: null,
      icon: Shield,
      iconColor: 'text-blue-400',
      valueColor: certExpiring > 0 ? 'text-rose-400' : 'text-white',
    },
  ]
})

onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  alertsStore.fetchAlerts()
  alertsStore.fetchActiveAlerts()
  if (hasFeature('incidents')) statusAdmin.fetchIncidents()
})

onUnmounted(() => {
  dashboard.disconnectAllSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-6">
      <div class="max-w-7xl mx-auto space-y-6 pb-12">

        <!-- Summary Cards -->
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
          <div
            v-for="card in summaryCards"
            :key="card.title"
            class="bg-[#151923] p-5 rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg group cursor-default"
          >
            <div class="flex justify-between items-start mb-4">
              <div class="p-2.5 bg-[#0f1115] rounded-xl group-hover:scale-105 transition-transform">
                <component :is="card.icon" :size="18" :class="card.iconColor" />
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
            <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">{{ card.title }}</p>
            <div class="flex items-baseline gap-2 mt-1">
              <h4 :class="['text-2xl font-black', card.valueColor]">{{ card.value }}</h4>
              <p class="text-[10px] text-slate-600 font-bold uppercase tracking-tight">{{ card.subtitle }}</p>
            </div>
          </div>
        </div>

        <!-- Update Summary Strip -->
        <UpdateSummaryStrip />

        <!-- Monitor Table -->
        <div class="bg-[#151923] rounded-2xl border border-slate-800 shadow-xl overflow-hidden">
          <!-- Table header -->
          <div class="px-6 py-5 border-b border-slate-800 flex justify-between items-center">
            <div>
              <h2 class="text-base font-bold text-white">Moniteurs unifiés</h2>
              <p class="text-xs text-slate-500 mt-0.5">Auto-découverte Docker et sondes externes</p>
            </div>
            <div class="flex items-center gap-2">
              <button
                @click="filterOpen = !filterOpen"
                :class="[
                  'px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all flex items-center gap-2 border',
                  hasActiveFilters
                    ? 'bg-blue-600/20 text-blue-400 border-blue-500/40 hover:bg-blue-600/30'
                    : 'bg-slate-800 hover:bg-slate-700 text-slate-200 border-slate-700',
                ]"
              >
                <Filter :size="13" />
                Filtrer
                <span v-if="hasActiveFilters" class="w-1.5 h-1.5 rounded-full bg-blue-400" />
              </button>
              <RouterLink
                to="/heartbeats"
                class="px-3.5 py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-bold transition-all flex items-center gap-2 shadow-lg shadow-blue-500/20"
              >
                <Zap :size="13" class="fill-white" />
                Ajouter un moniteur
              </RouterLink>
            </div>
          </div>

          <!-- Filter bar -->
          <div v-if="filterOpen" class="px-6 py-4 border-b border-slate-800 bg-[#0f1115]/40 flex flex-wrap items-center gap-3">
            <input
              v-model="filterSearch"
              type="text"
              placeholder="Rechercher un moniteur..."
              class="px-3 py-1.5 bg-[#0f1115] border border-slate-700 rounded-lg text-xs text-slate-200 placeholder-slate-600 focus:outline-none focus:border-blue-500 w-52"
            />
            <select
              v-model="filterType"
              class="px-3 py-1.5 bg-[#0f1115] border border-slate-700 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-blue-500 appearance-none cursor-pointer"
            >
              <option value="">Tous les types</option>
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
                  : 'bg-[#0f1115] text-slate-400 border-slate-700 hover:border-slate-600',
              ]"
            >
              Incidents uniquement
            </button>
            <button
              v-if="hasActiveFilters"
              @click="clearFilters"
              class="px-3 py-1.5 text-[10px] text-slate-500 hover:text-slate-300 font-bold uppercase tracking-widest transition-colors"
            >
              Effacer
            </button>
            <span class="ml-auto text-[10px] text-slate-600 font-bold">
              {{ filteredServices.length }} / {{ dashboard.monitors.length }} moniteurs
            </span>
          </div>

          <!-- Table -->
          <div class="overflow-x-auto">
            <table class="w-full text-left border-collapse">
              <thead>
                <tr class="bg-[#0f1115]/60 text-slate-500 text-[10px] uppercase tracking-widest font-bold border-b border-slate-800/60">
                  <th class="px-6 py-3.5">Statut / Nom</th>
                  <th class="px-6 py-3.5">Type</th>
                  <th class="px-6 py-3.5">Détails / Ressources</th>
                  <th class="px-6 py-3.5">Historique (90j)</th>
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
                        <p class="text-sm font-semibold text-slate-100 group-hover:text-blue-400 transition-colors truncate">
                          {{ service.name }}
                        </p>
                        <p class="text-[10px] text-slate-600 mt-0.5 flex items-center gap-1 truncate">
                          <Server v-if="service.type === 'container'" :size="9" />
                          <Clock v-else-if="service.type === 'heartbeat'" :size="9" />
                          <span>{{ service.subtitle }}</span>
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
                    <div v-else-if="service.metricValue" class="text-[10px] font-mono text-blue-400 font-bold">
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
                      <span>90j</span>
                      <span>Aujourd'hui</span>
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
                      <template v-if="dashboard.searchQuery || hasActiveFilters">Aucun moniteur correspondant aux filtres</template>
                      <template v-else>Aucun moniteur. Lancez des conteneurs Docker ou ajoutez des endpoints.</template>
                    </p>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Bottom Grid -->
        <div class="grid grid-cols-1 lg:grid-cols-3 gap-5">

          <!-- Incident Activity Feed -->
          <div class="lg:col-span-2 bg-[#151923] rounded-2xl border border-slate-800 p-6">
            <div class="flex justify-between items-center mb-5">
              <h3 class="text-sm font-bold text-white flex items-center gap-2.5">
                <Activity :size="15" class="text-blue-500" />
                Fil d'activité des incidents
              </h3>
              <RouterLink
                to="/alerts"
                class="text-[10px] text-blue-500 hover:text-blue-400 font-bold uppercase tracking-widest transition-colors"
              >
                Voir tout l'historique
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
                    <span class="text-xs font-semibold text-slate-200 group-hover:text-blue-400 transition-colors tracking-tight truncate mr-3">{{ inc.service }}</span>
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
              <p class="text-sm text-slate-600 font-medium">Aucun incident récent</p>
              <p class="text-[10px] text-slate-700">Tous les services fonctionnent normalement</p>
            </div>
          </div>

          <!-- Host Resources -->
          <div class="bg-[#151923] rounded-2xl border border-slate-800 p-6">
            <div class="flex items-center gap-2.5 mb-5">
              <Server :size="15" class="text-emerald-500" />
              <h3 class="text-sm font-bold text-white">Ressources Hôte</h3>
            </div>

            <div class="space-y-5">
              <!-- CPU -->
              <div class="space-y-1.5">
                <div class="flex justify-between items-center text-[10px] font-bold uppercase tracking-widest">
                  <span class="text-slate-500">Utilisation CPU</span>
                  <span class="text-slate-200">{{ Math.round(totalCpu) }}%</span>
                </div>
                <div class="h-1.5 w-full bg-[#0f1115] rounded-full border border-slate-800 overflow-hidden">
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
                  <span class="text-slate-500">Mémoire RAM</span>
                  <span class="text-slate-200 text-right">
                    {{ resources.formatBytes(totalMemUsed) }} / {{ resources.formatBytes(totalMemLimit) }}
                  </span>
                </div>
                <div class="h-1.5 w-full bg-[#0f1115] rounded-full border border-slate-800 overflow-hidden">
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
                  <span class="text-slate-500">Conteneurs</span>
                  <span class="text-slate-300 font-mono">{{ Object.keys(resources.snapshots).length }} actifs</span>
                </div>
                <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
                  <span class="text-slate-500">Moniteurs</span>
                  <span class="text-slate-300 font-mono">{{ dashboard.monitors.length }} total</span>
                </div>
                <div class="flex justify-between text-[10px] font-bold uppercase tracking-tight">
                  <span class="text-slate-500">Disponibilité</span>
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
        <div class="grid grid-cols-2 gap-3 mb-6">
          <div class="bg-[#0f1115] p-4 rounded-xl border border-slate-800">
            <p class="text-[10px] text-slate-500 font-bold uppercase mb-1.5 tracking-widest">Statut</p>
            <div class="flex items-center gap-2">
              <div :class="['w-2.5 h-2.5 rounded-full', statusDotClass(selectedService.status)]" />
              <p class="text-white font-semibold text-sm">{{ selectedService.statusLabel }}</p>
            </div>
          </div>
          <div v-if="selectedService.metricValue" class="bg-[#0f1115] p-4 rounded-xl border border-slate-800">
            <p class="text-[10px] text-slate-500 font-bold uppercase mb-1.5 tracking-widest">{{ selectedService.metricLabel || 'Métrique' }}</p>
            <p class="text-white font-semibold text-sm font-mono">{{ selectedService.metricValue }}</p>
          </div>
        </div>

        <div class="space-y-3 mb-6">
          <div>
            <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-2">Détails</h4>
            <div class="bg-[#0f1115] rounded-xl p-4 border border-slate-800 space-y-2.5">
              <div class="flex justify-between text-xs">
                <span class="text-slate-500">Type</span>
                <span class="text-slate-300 capitalize">{{ selectedService.type }}</span>
              </div>
              <div class="flex justify-between text-xs">
                <span class="text-slate-500">Source</span>
                <span class="text-slate-300 truncate ml-4 text-right">{{ selectedService.subtitle }}</span>
              </div>
              <div v-if="selectedService.group" class="flex justify-between text-xs">
                <span class="text-slate-500">Groupe</span>
                <span class="text-slate-300">{{ selectedService.group }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-if="selectedService.sparklineData && selectedService.sparklineData.length > 1" class="mb-6">
          <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-2">Tendance</h4>
          <div class="bg-[#0f1115] rounded-xl p-4 border border-slate-800">
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
            class="flex-1 py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-xl text-xs font-bold transition-all shadow-lg text-center"
            @click="slideOpen = false"
          >
            Voir les détails
          </RouterLink>
        </div>
      </template>
    </SlideOverPanel>
  </div>
</template>
