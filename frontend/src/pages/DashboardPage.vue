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
import { useDashboardStore } from '@/stores/dashboard'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { useResourcesStore } from '@/stores/resources'
import { usePostureStore } from '@/stores/posture'
import { useContainersStore } from '@/stores/containers'
import { useUpdatesStore } from '@/stores/updates'
import { useAgentsStore } from '@/stores/agents'
import { usePreferencesStore } from '@/stores/preferences'
import { useEdition } from '@/composables/useEdition'
import { useAttentionItems, type AttentionItem } from '@/composables/useAttentionItems'
import { useMonitorGroups } from '@/composables/useMonitorGroups'
import type { Severity } from '@/composables/useSeverity'
import type { GridItem } from '@/components/ui/statusGrid'
import type { KpiStripItem } from '@/components/ui/KpiStrip.vue'
import VerdictBanner from '@/components/dashboard/VerdictBanner.vue'
import AttentionPanel from '@/components/dashboard/AttentionPanel.vue'
import UpdateSummaryStrip from '@/components/dashboard/UpdateSummaryStrip.vue'
import RuntimeDegradedBanner from '@/components/RuntimeDegradedBanner.vue'
import StatusGrid from '@/components/ui/StatusGrid.vue'
import KpiStrip from '@/components/ui/KpiStrip.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { Activity, Zap, Cpu, Shield, ShieldCheck, Server } from 'lucide-vue-next'

const { hasFeature } = useEdition()
const router = useRouter()
const detailSlideOver = inject(detailSlideOverKey)!
const dashboard = useDashboardStore()
const resources = useResourcesStore()
const postureStore = usePostureStore()
const containersStore = useContainersStore()
const updatesStore = useUpdatesStore()
const agentsStore = useAgentsStore()
const prefs = usePreferencesStore()

const { items: attentionItems } = useAttentionItems()
const { monitorGroups } = useMonitorGroups()

const showPosture = computed(() => hasFeature('security_posture') && postureStore.posture !== null)
const stats = computed(() => dashboard.globalStats)

const SLIDEOVER_TYPES = new Set<string>(['container', 'heartbeat', 'certificate'])

const loading = ref(true)
const loadError = ref<string | null>(null)
const lastRefresh = ref(0)
const now = ref(Date.now())
let ticker: ReturnType<typeof setInterval> | null = null

// Persisted view preferences drive the StatusGrid + group-by toggle.
const view = computed<'grid' | 'list'>({
  get: () => prefs.monitorsView,
  set: (v) => {
    prefs.monitorsView = v
  },
})
const groupBy = computed<string>({
  get: () => prefs.monitorsGroupBy,
  set: (v) => {
    prefs.monitorsGroupBy = v === 'severity' ? 'severity' : 'type'
  },
})
const groupByOptions = [
  { value: 'type', label: 'By type' },
  { value: 'severity', label: 'By severity' },
]

// Human label [singular, plural] per attention kind, so the subtitle reads
// naturally and sums EXACTLY to the headline count. Updates are excluded from
// the breakdown (they are not in globalStats — they have their own strip).
const INCIDENT_LABEL: Record<string, [string, string]> = {
  Security: ['security risk', 'security risks'],
  Agent: ['agent disconnected', 'agents disconnected'],
  Endpoint: ['endpoint down', 'endpoints down'],
  Container: ['container down', 'containers down'],
  Heartbeat: ['heartbeat missed', 'heartbeats missed'],
  Certificate: ['certificate expired', 'certificates expired'],
  Workload: ['workload down', 'workloads down'],
}
const WARNING_LABEL: Record<string, [string, string]> = {
  Security: ['security warning', 'security warnings'],
  Endpoint: ['endpoint degraded', 'endpoints degraded'],
  Container: ['container unhealthy', 'containers unhealthy'],
  Heartbeat: ['heartbeat late', 'heartbeats late'],
  Certificate: ['certificate expiring', 'certificates expiring'],
  Workload: ['workload degraded', 'workloads degraded'],
}

const verdictSummary = computed(() => {
  if (stats.value.incidents === 0 && stats.value.warnings === 0) {
    const n = dashboard.monitors.length
    return n ? `${n} monitor${n > 1 ? 's' : ''} healthy` : 'No monitors configured yet'
  }
  // Break down the attention items of the headline severity by kind. Excludes
  // the warning-only "Update" kind so the parts sum to the header counter.
  const target = stats.value.incidents > 0 ? 'incident' : 'warning'
  const labels = target === 'incident' ? INCIDENT_LABEL : WARNING_LABEL
  const counts = new Map<string, number>()
  for (const it of attentionItems.value) {
    if (it.severity !== target || it.kind === 'Update') continue
    counts.set(it.kind, (counts.get(it.kind) ?? 0) + 1)
  }
  const parts: string[] = []
  for (const [kind, n] of counts) {
    const label = labels[kind] ?? [`${kind.toLowerCase()} ${target}`, `${kind.toLowerCase()} ${target}s`]
    parts.push(`${n} ${n > 1 ? label[1] : label[0]}`)
  }
  return parts.join(' · ')
})

const kpiStats = computed<KpiStripItem[]>(() => {
  const total = dashboard.monitors.length
  const uptime = total ? `${((stats.value.running / total) * 100).toFixed(1)}%` : '—'
  const eps = dashboard.monitors.filter((m) => m.type === 'endpoint' && m.metricValue)
  const lat = eps.length
    ? Math.round(eps.reduce((s, e) => s + (parseFloat(e.metricValue ?? '0') || 0), 0) / eps.length)
    : null
  const cpu = Math.round(resources.summary?.total_cpu_percent ?? 0)
  const mem = Math.round(resources.summary?.total_mem_percent ?? 0)

  const out: KpiStripItem[] = [
    { label: 'Global Uptime', value: uptime, sub: `${stats.value.running} / ${total} monitors`, icon: Activity },
    { label: 'Response Time', value: lat != null ? `${lat}ms` : 'N/A', sub: lat != null ? 'avg. endpoints' : 'no endpoints', icon: Zap },
    {
      label: 'Host CPU',
      value: `${cpu}%`,
      sub: `${mem}% memory`,
      icon: Cpu,
      tone: cpu > 80 ? 'incident' : cpu > 60 ? 'warning' : undefined,
    },
    { label: 'SSL Certificates', value: `${dashboard.certificateSummary.ok} OK`, sub: 'certificates', icon: Shield, to: { name: 'certificates' } },
  ]
  if (showPosture.value && postureStore.posture) {
    const c = postureStore.posture.color
    const tone: Severity = c === 'red' ? 'incident' : c === 'orange' || c === 'yellow' ? 'warning' : 'ok'
    out.push({
      label: 'Security Posture',
      value: String(postureStore.posture.score),
      sub: `${postureStore.posture.scored_count} scored`,
      icon: ShieldCheck,
      tone,
      to: { name: 'security' },
    })
  }
  return out
})

const freshness = computed(() => {
  if (!lastRefresh.value) return ''
  const s = Math.max(0, Math.floor((now.value - lastRefresh.value) / 1000))
  if (s < 10) return 'updated just now'
  if (s < 60) return `updated ${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `updated ${m}m ago`
  return `updated ${Math.floor(m / 60)}h ago`
})

function openMonitor(item: GridItem) {
  const [type, entityId] = item.id.split(':')
  if (type && SLIDEOVER_TYPES.has(type) && entityId) {
    detailSlideOver.openDetail(type as EntityType, entityId)
    return
  }
  const m = dashboard.monitors.find((mm) => mm.id === item.id)
  if (m) router.push(m.link)
}

function openAttention(item: AttentionItem) {
  if (item.nav.slideOver) {
    detailSlideOver.openDetail(item.nav.slideOver.type, item.nav.slideOver.id)
  } else if (item.nav.route) {
    router.push(item.nav.route)
  }
}

async function load() {
  loadError.value = null
  loading.value = true
  try {
    await dashboard.fetchAll()
  } catch {
    loadError.value = "Couldn't load monitors. Check the agent connection, then retry."
  } finally {
    loading.value = false
    lastRefresh.value = Date.now()
    now.value = Date.now()
  }
}

onMounted(() => {
  load()
  dashboard.connectAllSSE()
  updatesStore.fetchAllUpdates()
  updatesStore.fetchSummary()
  agentsStore.fetchAgents()
  if (hasFeature('security_posture')) postureStore.fetchPosture()
  ticker = setInterval(() => {
    now.value = Date.now()
  }, 15_000)
})

onUnmounted(() => {
  dashboard.disconnectAllSSE()
  if (ticker) clearInterval(ticker)
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="mx-auto max-w-7xl space-y-4 mnt-12 sm:space-y-6">
      <RuntimeDegradedBanner v-if="!containersStore.isContainerMonitoringAvailable" />

      <!-- 1. Verdict -->
      <VerdictBanner
        :incidents="stats.incidents"
        :warnings="stats.warnings"
        :ok="stats.running"
        :summary="verdictSummary"
      />

      <!-- 2. Needs attention -->
      <AttentionPanel :items="attentionItems" @select="openAttention" />

      <!-- 3. Overview -->
      <section>
        <SectionHeader title="Overview" class="mb-3" />
        <KpiStrip :stats="kpiStats" />
      </section>

      <UpdateSummaryStrip />

      <!-- 4. Monitors -->
      <section>
        <div v-if="loadError" class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface">
          <ErrorState :message="loadError" retryable @retry="load" />
        </div>

        <div v-else-if="loading && dashboard.monitors.length === 0" class="space-y-3">
          <SectionHeader title="Monitors" />
          <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
            <LoadingSkeleton variant="grid" :count="18" />
          </div>
        </div>

        <StatusGrid v-else v-model:view="view" :groups="monitorGroups" @select="openMonitor">
          <template #bar>
            <div class="flex items-center gap-2.5">
              <h2 class="text-xs font-semibold uppercase tracking-wide text-mnt-muted">Monitors</h2>
              <span class="rounded-full bg-mnt-elevated px-2 py-0.5 font-mono text-[11px] text-mnt-muted">
                {{ dashboard.monitors.length }}
              </span>
              <span v-if="freshness" class="hidden text-[11px] text-mnt-muted sm:inline">· {{ freshness }}</span>
            </div>
            <SegmentedToggle v-model="groupBy" :options="groupByOptions" ariaLabel="Group monitors by" />
          </template>
          <template #empty>
            <EmptyState
              :icon="Server"
              title="No monitors yet"
              description="Start containers, deploy workloads, or add endpoints to see them here."
            />
          </template>
        </StatusGrid>
      </section>
    </div>
  </div>
</template>
