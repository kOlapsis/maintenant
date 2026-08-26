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
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardStore } from '@/stores/dashboard'
import { useAlertsStore } from '@/stores/alerts'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import { useStorageStore } from '@/stores/storage'
import { useAgentsStore } from '@/stores/agents'
import { Search, Bell, AlertTriangle, Box, Globe, Heart, ShieldCheck, Cpu, Sun, Moon, Monitor, MessageSquare } from 'lucide-vue-next'
import RuntimeBadge from '@/components/RuntimeBadge.vue'
import HostFilterDropdown from '@/components/HostFilterDropdown.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import { useTheme } from '@/composables/useTheme'
import { useFeedbackUrl } from '@/composables/useFeedbackUrl'

const router = useRouter()
const dashboard = useDashboardStore()
const alertsStore = useAlertsStore()
const resources = useResourcesStore()
const containers = useContainersStore()
const storage = useStorageStore()
const agentsStore = useAgentsStore()

// Active enrolled agents drive the global host filter (the dropdown lives in the
// header, before the search); this watch keeps the selection valid when agents change.
const activeAgentIds = computed(() =>
  agentsStore.agents.filter((a) => a.status === 'active').map((a) => a.agent_id),
)

// Clear the selection if the selected agent was revoked/deleted (fires when the
// agent list changes, e.g. after the initial fetch or a refresh).
watch(activeAgentIds, (ids) => resources.reconcile(new Set(ids)))

// Refetch every list + the header gauges whenever the global host filter changes.
watch(
  () => resources.selected,
  () => {
    dashboard.refetchForFilter()
    resources.fetchSummary()
  },
)

let summaryInterval: ReturnType<typeof setInterval> | null = null

// Global SSE connections + initial data fetch — always active while the app shell is mounted
onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  agentsStore.fetchAgents()
  resources.fetchSummary()
  summaryInterval = setInterval(() => {
    agentsStore.fetchAgents()
    resources.fetchSummary()
  }, 30_000)
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
  if (value >= 90) return 'var(--mnt-status-down-text)'
  if (value >= 70) return 'var(--mnt-status-warn-text)'
  return 'var(--mnt-status-ok-text)'
}

const { theme, setTheme } = useTheme()

const themeOrder = ['system', 'light', 'dark'] as const

function cycleTheme() {
  const idx = themeOrder.indexOf(theme.value)
  setTheme(themeOrder[(idx + 1) % themeOrder.length]!)
}

const themeTooltip = computed(() => {
  if (theme.value === 'light') return 'Light mode'
  if (theme.value === 'dark') return 'Dark mode'
  return 'Follow system theme'
})

const { feedbackUrl } = useFeedbackUrl()
</script>

<template>
  <header class="header-glass hidden md:flex h-16 shrink-0 border-b border-mnt-default items-center justify-between px-6 backdrop-blur-md z-10">
    <div class="flex items-center gap-5">
      <!-- Global host/resource scope selector (hidden on single-host installs) -->
      <HostFilterDropdown v-if="activeAgentIds.length > 0" class="w-48 shrink-0" />

      <!-- Search -->
      <div class="relative group">
        <Search
          :size="15"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-mnt-muted group-focus-within:text-mnt-green-400 transition-colors"
        />
        <input
          v-model="dashboard.searchQuery"
          type="text"
          placeholder="Search services..."
          class="bg-mnt-primary border border-mnt-default rounded-lg py-2 pl-9 pr-4 text-sm w-72 focus:outline-none focus:ring-1 focus:ring-mnt-green-500/60 focus:border-mnt-green-500/40 transition-all text-mnt-primary placeholder:text-mnt-muted"
        />
      </div>

      <!-- Monitor health counters (containers + endpoints + heartbeats + certificates) -->
      <div class="hidden sm:flex items-center gap-5 border-l border-mnt-default pl-5">
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">OK</span>
          <span class="text-sm font-black text-mnt-status-ok">{{ dashboard.globalStats.running }}</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">Warning</span>
          <span
            class="text-sm font-black"
            :class="dashboard.globalStats.warnings > 0 ? 'text-mnt-status-warn' : 'text-mnt-muted'"
          >{{ dashboard.globalStats.warnings }}</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">Incident</span>
          <span
            class="text-sm font-black"
            :class="dashboard.globalStats.incidents > 0 ? 'text-mnt-status-down' : 'text-mnt-muted'"
          >{{ dashboard.globalStats.incidents }}</span>
        </div>
      </div>

      <!-- Resource gauges -->
      <div class="hidden lg:flex items-center gap-4 border-l border-mnt-default pl-5">
        <!-- CPU -->
        <div class="flex items-center gap-2 min-w-[120px]">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest w-8">CPU</span>
          <div class="flex-1 h-1.5 rounded-full bg-mnt-elevated overflow-hidden">
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
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest w-8">MEM</span>
          <div class="flex-1 h-1.5 rounded-full bg-mnt-elevated overflow-hidden">
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
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest w-8">DISK</span>
          <div class="flex-1 h-1.5 rounded-full bg-mnt-elevated overflow-hidden">
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

    <!-- Right: runtime badge + theme toggle + feedback + bell -->
    <div class="flex items-center gap-4">
      <!-- Runtime badge with popover -->
      <RuntimeBadge />

      <!-- Theme toggle -->
      <button
        @click="cycleTheme"
        :title="themeTooltip"
        :aria-label="themeTooltip"
        class="p-2 text-mnt-muted hover:text-mnt-primary hover:bg-mnt-elevated rounded-lg transition-all"
      >
        <Sun v-if="theme === 'light'" :size="18" />
        <Moon v-else-if="theme === 'dark'" :size="18" />
        <Monitor v-else :size="18" />
      </button>

      <!-- Feedback form on maintenant.dev, opened by the browser in a new tab -->
      <a
        :href="feedbackUrl"
        target="_blank"
        rel="noopener noreferrer"
        title="Give feedback"
        aria-label="Give feedback"
        class="p-2 text-mnt-muted hover:text-mnt-primary hover:bg-mnt-elevated rounded-lg transition-all"
      >
        <MessageSquare :size="18" />
      </a>

      <div
        class="relative"
        @mouseenter="onBellEnter"
        @mouseleave="onBellLeave"
      >
        <button
          @click="onBellClick"
          :aria-label="alertsStore.totalActiveCount > 0 ? `View alerts (${alertsStore.totalActiveCount} active)` : 'View alerts'"
          class="p-2 text-mnt-muted hover:text-mnt-primary hover:bg-mnt-elevated rounded-lg transition-all relative"
        >
          <Bell :size="18" />
          <span
            v-if="alertsStore.totalActiveCount > 0"
            class="absolute top-1.5 right-1.5 h-2 w-2 rounded-full"
            :style="{ backgroundColor: alertsStore.activeAlerts.critical.length > 0 ? 'var(--mnt-sev-incident)' : 'var(--mnt-sev-warning)' }"
          >
            <span
              class="mnt-ping absolute inset-0 rounded-full"
              :style="{ backgroundColor: alertsStore.activeAlerts.critical.length > 0 ? 'var(--mnt-sev-incident)' : 'var(--mnt-sev-warning)' }"
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
            class="absolute right-0 top-full mt-2 w-56 rounded-xl border border-mnt-default bg-mnt-surface shadow-2xl shadow-black/40 overflow-hidden z-50"
            @mouseenter="onBellEnter"
            @mouseleave="onBellLeave"
          >
            <div class="px-3 py-2.5 border-b border-mnt-default flex items-center justify-between">
              <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">Active alerts</span>
              <span
                class="min-w-[20px] h-5 flex items-center justify-center rounded-full text-[10px] font-bold px-1.5"
                :class="alertsStore.activeAlerts.critical.length > 0 ? 'bg-mnt-status-down text-mnt-status-down' : 'bg-mnt-status-warn text-mnt-status-warn'"
              >
                {{ alertsStore.totalActiveCount }}
              </span>
            </div>
            <div class="py-1">
              <button
                v-for="source in sourceKeys"
                :key="source"
                @click="navigateToSource(source)"
                class="w-full flex items-center gap-3 px-3 py-2 text-sm text-mnt-secondary hover:bg-mnt-elevated transition-colors"
              >
                <component
                  :is="sourceRouteMap[source]?.icon ?? AlertTriangle"
                  :size="14"
                  class="shrink-0"
                  :class="alertsBySource[source]?.critical ? 'text-mnt-status-down' : alertsBySource[source]?.warning ? 'text-mnt-status-warn' : 'text-mnt-green-400'"
                />
                <span class="flex-1 text-left">{{ sourceRouteMap[source]?.label ?? source }}</span>
                <span
                  class="min-w-[20px] h-5 flex items-center justify-center rounded-full text-[10px] font-bold px-1.5"
                  :class="alertsBySource[source]?.critical ? 'bg-mnt-status-down text-mnt-status-down' : alertsBySource[source]?.warning ? 'bg-mnt-status-warn text-mnt-status-warn' : 'bg-mnt-green-500/15 text-mnt-green-400'"
                >
                  {{ alertsBySource[source]?.count }}
                </span>
              </button>
            </div>
            <div class="border-t border-mnt-default">
              <button
                @click="navigateToSource('_all')"
                class="w-full px-3 py-2 text-[11px] font-medium text-mnt-muted hover:text-mnt-secondary hover:bg-mnt-elevated transition-colors text-center"
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
  <AlertBanner
    v-if="!containers.runtimeConnected"
    severity="critical"
    label="RUNTIME OFFLINE"
  >
    <strong class="font-semibold">{{ containers.runtimeLabel }}</strong> runtime disconnected — monitoring paused until connection is restored.
  </AlertBanner>

  <!-- Storage outage banner: the screens keep whatever they already know
       rather than emptying out, and say why they are not refreshing. -->
  <AlertBanner
    v-if="!storage.connected"
    severity="critical"
    label="STORAGE OFFLINE"
  >
    Database unreachable — what you see may be out of date. Monitoring resumes on its own once the database answers again.
  </AlertBanner>
</template>

<style scoped>
/* `bg-mnt-*` are hand-written utilities: Tailwind's `/60` opacity modifier
   does not apply to them, so mix the token here. */
.header-glass {
  background-color: color-mix(in srgb, var(--mnt-bg-surface) 60%, transparent);
}
</style>
