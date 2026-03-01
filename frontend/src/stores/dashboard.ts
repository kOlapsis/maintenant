import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useContainersStore } from './containers'
import { useEndpointsStore } from './endpoints'
import { useHeartbeatsStore } from './heartbeats'
import { useCertificatesStore } from './certificates'
import { useAlertsStore } from './alerts'
import { useResourcesStore } from './resources'
import { apiFetch } from '@/services/apiFetch'
import type { Container } from '@/services/containerApi'
import type { Endpoint } from '@/services/endpointApi'
import type { Heartbeat } from '@/services/heartbeatApi'
import type { CertMonitor } from '@/services/certificateApi'
import type { IncidentTimelineEntry } from '@/components/dashboard/IncidentBanner.vue'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export type UnifiedStatus = 'ok' | 'warning' | 'down' | 'paused' | 'unknown'
export type MonitorType = 'container' | 'endpoint' | 'heartbeat' | 'certificate'

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
  link: { name: string; params: Record<string, string> }
  updatedAt: string
}

function containerStatus(c: Container): { status: UnifiedStatus; label: string } {
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
        link: { name: 'containers', params: {} },
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

  async function fetchAll() {
    await Promise.all([
      containers.fetchContainers(),
      endpoints.fetchEndpoints(),
      heartbeats.fetchHeartbeats(),
      certificates.fetchCertificates(),
      alertsStore.fetchActiveAlerts(),
      fetchSparklines(),
    ])
  }

  function connectAllSSE() {
    containers.connectSSE()
    endpoints.connectSSE()
    heartbeats.connectSSE()
    certificates.connectSSE()
    alertsStore.connectSSE()
    resourcesStore.connectSSE()

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
    // Use active alerts as the incident count — each active alert already
    // represents a unique incident (endpoint down, cert error, etc.).
    // Adding "down" monitors on top would double-count since they also
    // generate an active alert.
    const activeAlertCount =
      (alertsStore.activeAlerts.critical?.length ?? 0) +
      (alertsStore.activeAlerts.warning?.length ?? 0) +
      (alertsStore.activeAlerts.info?.length ?? 0)
    return { running, incidents: activeAlertCount, warnings: warning }
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
