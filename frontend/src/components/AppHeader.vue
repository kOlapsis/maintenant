<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
import { useVisibleInterval } from '@/composables/useVisibleInterval'
import { Bell, AlertTriangle, Box, Globe, Heart, ShieldCheck, Cpu, Sun, Moon, Monitor, MessageSquare } from 'lucide-vue-next'
import RuntimeBadge from '@/components/RuntimeBadge.vue'
import HostFilterDropdown from '@/components/HostFilterDropdown.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import PopoverMenu from '@/components/ui/PopoverMenu.vue'
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

// Global SSE connections + initial data fetch — always active while the app shell is mounted
onMounted(() => {
  dashboard.fetchAll()
  dashboard.connectAllSSE()
  agentsStore.fetchAgents()
  resources.fetchSummary()
})

useVisibleInterval(() => {
  agentsStore.fetchAgents()
  resources.fetchSummary()
}, 30_000)

onUnmounted(() => {
  dashboard.disconnectAllSSE()
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

const themeIcon = computed(() => {
  if (theme.value === 'light') return Sun
  if (theme.value === 'dark') return Moon
  return Monitor
})

const { feedbackUrl } = useFeedbackUrl()
</script>

<template>
  <header class="header-glass @container hidden md:flex h-16 shrink-0 border-b border-mnt-default items-center justify-between gap-4 px-6 backdrop-blur-md z-10">
    <div class="flex min-w-0 flex-1 items-center gap-5 overflow-hidden">
      <!-- Global host/resource scope selector (hidden on single-host installs) -->
      <HostFilterDropdown v-if="activeAgentIds.length > 0" class="w-48 shrink-0" />

      <!-- Search -->
      <SearchInput v-model="dashboard.searchQuery" placeholder="Search services..." class="w-72 min-w-40 shrink" />

      <!-- Monitor health counters (containers + endpoints + heartbeats + certificates) -->
      <div
        class="hidden shrink-0 items-center gap-5 border-l border-mnt-default pl-5"
        :class="activeAgentIds.length > 0 ? '@min-[59rem]:flex' : '@min-[45rem]:flex'"
      >
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
      <div
        class="hidden shrink-0 items-center gap-4 border-l border-mnt-default pl-5"
        :class="activeAgentIds.length > 0 ? '@min-[86rem]:flex' : '@min-[72rem]:flex'"
      >
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
    <div class="flex shrink-0 items-center gap-4">
      <!-- Runtime badge with popover -->
      <RuntimeBadge />

      <!-- Theme toggle -->
      <UiButton
        variant="ghost"
        size="sm"
        :icon="themeIcon"
        :title="themeTooltip"
        :aria-label="themeTooltip"
        @click="cycleTheme"
      />

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

      <PopoverMenu
        v-model:open="bellOpen"
        ariaLabel="Active alerts"
        panel-class="w-56"
        @mouseenter="onBellEnter"
        @mouseleave="onBellLeave"
      >
        <template #trigger>
          <UiButton
            variant="ghost"
            size="sm"
            :icon="Bell"
            class="relative"
            :aria-label="alertsStore.totalActiveCount > 0 ? `View alerts (${alertsStore.totalActiveCount} active)` : 'View alerts'"
            @click="onBellClick"
          >
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
          </UiButton>
        </template>

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
          <UiButton
            v-for="source in sourceKeys"
            :key="source"
            variant="ghost"
            block
            align="start"
            role="menuitem"
            class="gap-3 rounded-none px-3 py-2 text-sm text-mnt-secondary hover:text-mnt-secondary"
            @click="navigateToSource(source)"
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
          </UiButton>
        </div>
        <div class="border-t border-mnt-default">
          <UiButton
            variant="ghost"
            block
            role="menuitem"
            class="rounded-none px-3 py-2 text-[11px] font-medium text-mnt-muted hover:text-mnt-secondary"
            @click="navigateToSource('_all')"
          >
            View all alerts
          </UiButton>
        </div>
      </PopoverMenu>
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
