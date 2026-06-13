// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { useContainersStore } from './containers'
import { useEndpointsStore } from './endpoints'
import { useHeartbeatsStore } from './heartbeats'
import { useCertificatesStore } from './certificates'
import { useAlertsStore } from './alerts'
import { useResourcesStore } from './resources'
import { useKubernetesStore } from './kubernetes'
import { useFleetRuntimes } from '@/composables/useFleetRuntimes'
import { apiFetch } from '@/services/apiFetch'
import { sseBus } from '@/services/sseBus'
import type { Container } from '@/services/containerApi'
import type { Endpoint } from '@/services/endpointApi'
import type { Heartbeat } from '@/services/heartbeatApi'
import type { CertMonitor } from '@/services/certificateApi'
import type { K8sWorkload } from '@/services/kubernetesApi'
import type { IncidentTimelineEntry } from '@/components/dashboard/IncidentBanner.vue'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export type UnifiedStatus = 'ok' | 'warning' | 'down' | 'paused' | 'unknown'
export type MonitorType = 'container' | 'endpoint' | 'heartbeat' | 'certificate' | 'workload'

export interface UnifiedMonitor {
  id: string
  type: MonitorType
  name: string
  status: UnifiedStatus
  statusLabel: string
  subtitle: string
  group: string | null
  sparklineData: number[] | null
  sparklineType: 'latency' | 'uptime' | 'cpu' | null
  metricValue: string | null
  metricLabel: string | null
  link: { name: string; params?: Record<string, string>; query?: Record<string, string> }
  updatedAt: string
}

function containerStatus(c: Container): { status: UnifiedStatus; label: string } {
  if (c.stale) return { status: 'unknown', label: 'Agent offline' }
  if (c.state === 'paused') return { status: 'paused', label: 'Paused' }
  if (c.state === 'completed') return { status: 'paused', label: 'Completed' }
  if (c.state === 'running') {
    if (c.has_health_check && c.health_status === 'unhealthy')
      return { status: 'warning', label: 'Unhealthy' }
    return { status: 'ok', label: c.health_status === 'healthy' ? 'Healthy' : 'Running' }
  }
  return { status: 'down', label: c.state === 'exited' ? 'Exited' : c.state || 'Down' }
}

function endpointStatus(e: Endpoint): { status: UnifiedStatus; label: string } {
  if (e.stale) return { status: 'unknown', label: 'Agent offline' }
  if (e.status === 'up') return { status: 'ok', label: 'Up' }
  if (e.status === 'down') return { status: 'down', label: 'Down' }
  return { status: 'unknown', label: 'Unknown' }
}

function heartbeatStatus(h: Heartbeat): { status: UnifiedStatus; label: string } {
  if (h.status === 'paused') return { status: 'paused', label: 'Paused' }
  if (h.status === 'up') return { status: 'ok', label: 'On Time' }
  if (h.status === 'started') return { status: 'ok', label: 'Started' }
  if (h.status === 'down') return { status: 'down', label: 'Missed' }
  return { status: 'unknown', label: 'New' }
}

function certStatus(c: CertMonitor): { status: UnifiedStatus; label: string } {
  const days = c.latest_check?.days_remaining
  if (days != null) {
    if (days < 3) return { status: 'down', label: `${days}d left` }
    if (days < 30) return { status: 'warning', label: `${days}d left` }
    return { status: 'ok', label: `${days}d left` }
  }
  if (c.status === 'valid') return { status: 'ok', label: 'Valid' }
  if (c.status === 'expiring') return { status: 'warning', label: 'Expiring soon' }
  if (c.status === 'expired') return { status: 'down', label: 'Expired' }
  if (c.status === 'error') return { status: 'down', label: 'Error' }
  return { status: 'unknown', label: 'Pending' }
}

// A K8s workload is the cluster analogue of a Docker container / Swarm service —
// "the service you monitor" — so it maps onto the same unified status set.
function workloadStatus(w: K8sWorkload): { status: UnifiedStatus; label: string } {
  // The reporting agent is offline — its last-known status is no longer live.
  if (w.stale) return { status: 'unknown', label: 'Agent offline' }
  switch (w.status) {
    case 'healthy':
      return { status: 'ok', label: 'Healthy' }
    case 'progressing':
      return { status: 'warning', label: 'Progressing' }
    case 'degraded':
      return { status: 'warning', label: 'Degraded' }
    case 'failed':
      return { status: 'down', label: 'Failed' }
    default:
      return { status: 'unknown', label: 'Unknown' }
  }
}

const statusOrder: Record<UnifiedStatus, number> = {
  down: 0,
  warning: 1,
  unknown: 2,
  ok: 3,
  paused: 4,
}

export const useDashboardStore = defineStore('dashboard', () => {
  const containers = useContainersStore()
  const endpoints = useEndpointsStore()
  const heartbeats = useHeartbeatsStore()
  const certificates = useCertificatesStore()
  const alertsStore = useAlertsStore()
  const resourcesStore = useResourcesStore()
  const kubernetes = useKubernetesStore()
  const { availableRuntimes } = useFleetRuntimes()

  // Kubernetes contributes workloads (not containers); only surface them when the
  // selected host scope actually offers a Kubernetes runtime, so a Docker-only
  // install never queries the K8s endpoints.
  const kubernetesInScope = computed(() => availableRuntimes.value.includes('kubernetes'))

  const searchQuery = ref('')

  const sparklines = ref<Record<string, number[]>>({})
  let sparklineInterval: ReturnType<typeof setInterval> | null = null

  async function fetchSparklines() {
    try {
      const data = await apiFetch<Record<string, number[]>>(`${API_BASE}/dashboard/sparklines`)
      sparklines.value = data
    } catch {
      // ignore — sparklines are non-critical
    }
  }

  const monitors = computed<UnifiedMonitor[]>(() => {
    const result: UnifiedMonitor[] = []

    for (const c of containers.activeContainers) {
      // One-shot containers (migrations, seeds) that exited normally
      // are not services to monitor — exclude from dashboard
      if (c.state === 'completed') continue

      const s = containerStatus(c)
      const snap = resourcesStore.snapshots[c.id]
      const cpuSpark = resourcesStore.cpuSparklines[c.id]
      result.push({
        id: `container:${c.id}`,
        type: 'container',
        name: c.name,
        status: s.status,
        statusLabel: s.label,
        subtitle: c.image.split('@')[0] ?? c.image,
        group: c.orchestration_group || c.custom_group || null,
        sparklineData: cpuSpark?.length ? cpuSpark : null,
        sparklineType: cpuSpark?.length ? 'cpu' : null,
        metricValue: snap ? `${snap.cpu_percent.toFixed(1)}%` : null,
        metricLabel: snap ? 'cpu' : null,
        link: { name: 'containers', query: { selected: String(c.id) } },
        updatedAt: c.last_state_change_at,
      })
    }

    for (const e of endpoints.endpoints) {
      const s = endpointStatus(e)
      const epSparkline = sparklines.value[`endpoint:${e.id}`]
      result.push({
        id: `endpoint:${e.id}`,
        type: 'endpoint',
        name: e.target,
        status: s.status,
        statusLabel: s.label,
        subtitle: `${e.endpoint_type.toUpperCase()} - ${e.container_name}`,
        group: e.orchestration_group || null,
        sparklineData: epSparkline?.length ? epSparkline : null,
        sparklineType: 'latency',
        metricValue: e.last_response_time_ms != null ? `${e.last_response_time_ms}ms` : null,
        metricLabel: e.last_response_time_ms != null ? 'latency' : null,
        link: { name: 'endpoints', params: {} },
        updatedAt: e.last_check_at || e.first_seen_at,
      })
    }

    for (const h of heartbeats.heartbeats) {
      const s = heartbeatStatus(h)
      result.push({
        id: `heartbeat:${h.id}`,
        type: 'heartbeat',
        name: h.name,
        status: s.status,
        statusLabel: s.label,
        subtitle: `Every ${formatInterval(h.interval_seconds)}`,
        group: null,
        sparklineData: null,
        sparklineType: null,
        metricValue: h.last_duration_ms != null ? `${h.last_duration_ms}ms` : null,
        metricLabel: h.last_duration_ms != null ? 'duration' : null,
        link: { name: 'heartbeats', params: {} },
        updatedAt: h.last_ping_at || h.created_at,
      })
    }

    for (const c of certificates.certificates) {
      const s = certStatus(c)
      const days = c.latest_check?.days_remaining
      result.push({
        id: `certificate:${c.id}`,
        type: 'certificate',
        name: c.hostname,
        status: s.status,
        statusLabel: s.label,
        subtitle: c.latest_check?.issuer_cn || 'Pending check',
        group: null,
        sparklineData: null,
        sparklineType: null,
        metricValue: days != null ? `${days}d` : null,
        metricLabel: days != null ? 'expires' : null,
        link: { name: 'certificates', params: {} },
        updatedAt: c.last_check_at || c.created_at,
      })
    }

    if (kubernetesInScope.value) {
      for (const group of kubernetes.workloadGroups) {
        for (const w of group.workloads) {
          const s = workloadStatus(w)
          result.push({
            id: `workload:${w.id}`,
            type: 'workload',
            name: w.name,
            status: s.status,
            statusLabel: s.label,
            subtitle: `${w.kind} · ${w.namespace}`,
            group: w.namespace,
            sparklineData: null,
            sparklineType: null,
            metricValue: `${w.ready_replicas}/${w.desired_replicas}`,
            metricLabel: 'ready',
            link: { name: 'workloads' },
            updatedAt: w.last_transition || w.created_at,
          })
        }
      }
    }

    return result
  })

  const monitorsByType = computed(() => {
    const grouped = new Map<MonitorType, UnifiedMonitor[]>()
    for (const m of monitors.value) {
      const list = grouped.get(m.type) || []
      list.push(m)
      grouped.set(m.type, list)
    }
    // Sort each group: problems first
    for (const [, list] of grouped) {
      list.sort((a, b) => statusOrder[a.status] - statusOrder[b.status])
    }
    return grouped
  })

  function summaryCounts(type: MonitorType) {
    const list = monitorsByType.value.get(type) || []
    let ok = 0, warning = 0, down = 0
    for (const m of list) {
      if (m.status === 'ok') ok++
      else if (m.status === 'warning') warning++
      else if (m.status === 'down') down++
    }
    return { total: list.length, ok, warning, down }
  }

  const containerSummary = computed(() => summaryCounts('container'))
  const endpointSummary = computed(() => summaryCounts('endpoint'))
  const heartbeatSummary = computed(() => summaryCounts('heartbeat'))
  const certificateSummary = computed(() => summaryCounts('certificate'))

  const activeIncidents = computed<IncidentTimelineEntry[]>(() => {
    const active = alertsStore.activeAlerts
    const incidents: IncidentTimelineEntry[] = []
    for (const severity of ['critical', 'warning', 'info'] as const) {
      for (const alert of active[severity]) {
        incidents.push({
          id: alert.id,
          monitorType: alert.entity_type || '',
          monitorName: alert.entity_name || `Alert #${alert.id}`,
          severity,
          message: alert.message || '',
          states: [{ status: 'detected', timestamp: alert.fired_at || '', actor: null }],
          isActive: true,
        })
      }
    }
    return incidents
  })

  function fetchWorkloadsIfInScope() {
    return kubernetesInScope.value ? kubernetes.fetchWorkloadsList() : Promise.resolve()
  }

  async function fetchAll() {
    await Promise.all([
      containers.fetchContainers(),
      endpoints.fetchEndpoints(),
      heartbeats.fetchHeartbeats(),
      certificates.fetchCertificates(),
      alertsStore.fetchActiveAlerts(),
      fetchSparklines(),
      fetchWorkloadsIfInScope(),
    ])
  }

  // refetchForFilter re-fetches every entity list so they pick up the current
  // global host filter (resources.entityQuery). Called by resources.setFilter.
  async function refetchForFilter() {
    await Promise.all([
      containers.fetchContainers(),
      endpoints.fetchEndpoints(),
      heartbeats.fetchHeartbeats(),
      certificates.fetchCertificates(),
      fetchWorkloadsIfInScope(),
    ])
  }

  // The fleet only learns it has a Kubernetes runtime once the agent list loads,
  // which can race the initial fetchAll() (e.g. a persisted K8s-agent filter on a
  // fresh page load). Backfill the workloads when K8s enters scope.
  watch(kubernetesInScope, (inScope) => {
    if (inScope) kubernetes.fetchWorkloadsList()
  })

  // Dashboard-owned K8s liveness. We register our OWN handlers (distinct refs from
  // the kubernetes store's startListening singletons) so a K8s page unmounting and
  // calling stopListening can't tear down the dashboard's refresh.
  function onWorkloadTopologyChanged() {
    if (kubernetesInScope.value) kubernetes.fetchWorkloadsList()
  }

  function connectAllSSE() {
    containers.connectSSE()
    endpoints.connectSSE()
    heartbeats.connectSSE()
    certificates.connectSSE()
    alertsStore.connectSSE()
    resourcesStore.connectSSE()
    sseBus.on('kubernetes.workload_changed', onWorkloadTopologyChanged)
    sseBus.on('kubernetes.topology_changed', onWorkloadTopologyChanged)

    // Refresh sparklines every 60s
    if (!sparklineInterval) {
      sparklineInterval = setInterval(fetchSparklines, 60_000)
    }
  }

  function disconnectAllSSE() {
    containers.disconnectSSE()
    endpoints.disconnectSSE()
    heartbeats.disconnectSSE()
    certificates.disconnectSSE()
    alertsStore.disconnectSSE()
    resourcesStore.disconnectSSE()
    sseBus.off('kubernetes.workload_changed', onWorkloadTopologyChanged)
    sseBus.off('kubernetes.topology_changed', onWorkloadTopologyChanged)

    if (sparklineInterval) {
      clearInterval(sparklineInterval)
      sparklineInterval = null
    }
  }

  const globalStats = computed(() => {
    let running = 0, down = 0, warning = 0
    for (const m of monitors.value) {
      if (m.status === 'ok') running++
      else if (m.status === 'down') down++
      else if (m.status === 'warning') warning++
    }
    // Split alert severities: incidents = critical alerts, warnings = warning
    // alerts. This avoids the confusing mix where restart-loop warnings were
    // counted as "incidents" while the header "WARNINGS" counter only showed
    // health-check unhealthy monitors.
    const incidents = alertsStore.activeAlerts.critical?.length ?? 0
    const warningAlerts = alertsStore.activeAlerts.warning?.length ?? 0
    return { running, incidents, warnings: warning + warningAlerts }
  })

  return {
    monitors,
    monitorsByType,
    searchQuery,
    globalStats,
    containerSummary,
    endpointSummary,
    heartbeatSummary,
    certificateSummary,
    activeIncidents,
    fetchAll,
    refetchForFilter,
    connectAllSSE,
    disconnectAllSSE,
  }
})

function formatInterval(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
  return `${Math.round(seconds / 86400)}d`
}
