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
import { ref, computed } from 'vue'
import {
  getSummary,
  type ResourceSnapshot,
  type ResourceSummary,
} from '@/services/resourceApi'
import { sseBus } from '@/services/sseBus'

// Persisted global host filter: null = all resources, 'local' = the server's own
// runtime, '<agent_id>' = a specific enrolled agent.
const FILTER_KEY = 'pb:host-filter:selected'

export interface ResourceAlert {
  container_id: string
  container_name: string
  alert_type: string
  current_value: number
  threshold: number
  timestamp: string
}

const SPARKLINE_BUFFER_SIZE = 20

export const useResourcesStore = defineStore('resources', () => {
  const snapshots = ref<Record<string, ResourceSnapshot>>({})
  const alerts = ref<Record<string, ResourceAlert>>({})
  const summary = ref<ResourceSummary | null>(null)
  const cpuSparklines = ref<Record<string, number[]>>({})

  // Global host filter — the single source of truth that scopes every list
  // (containers, endpoints, certificates, heartbeats) and the dashboard.
  // null = all resources, 'local' = the server's own runtime, '<id>' = an agent.
  const selected = ref<string | null>(localStorage.getItem(FILTER_KEY) || null)

  // Query value for entity lists: passes 'local'/'<id>' through verbatim (the
  // backend understands both); null → undefined → no filter (all resources).
  const entityQuery = computed<string | undefined>(() => selected.value ?? undefined)

  // Query value for the per-host resource summary (header gauges). There is no
  // multi-host aggregate, so 'all' falls back to the local server, same as 'local'.
  const summaryQuery = computed<string | undefined>(() =>
    selected.value === null || selected.value === 'local' ? undefined : selected.value,
  )

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024))
    const idx = Math.min(i, units.length - 1)
    return `${(bytes / Math.pow(1024, idx)).toFixed(idx > 0 ? 1 : 0)} ${units[idx]}`
  }

  function formatPercent(value: number): string {
    return `${value.toFixed(1)}%`
  }

  const getSnapshot = computed(() => {
    return (containerId: string) => snapshots.value[containerId] || null
  })

  const getAlert = computed(() => {
    return (containerId: string) => alerts.value[containerId] || null
  })

  const formattedSnapshot = computed(() => {
    return (containerId: string) => {
      const snap = snapshots.value[containerId]
      if (!snap) return null
      return {
        cpu: formatPercent(snap.cpu_percent),
        memUsed: formatBytes(snap.mem_used),
        memLimit: formatBytes(snap.mem_limit),
        memPercent: formatPercent(snap.mem_percent),
        netRx: formatBytes(snap.net_rx_bytes),
        netTx: formatBytes(snap.net_tx_bytes),
        blockRead: formatBytes(snap.block_read_bytes),
        blockWrite: formatBytes(snap.block_write_bytes),
      }
    }
  })

  function onSnapshot(e: MessageEvent) {
    let data: ResourceSnapshot
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    snapshots.value[data.container_id] = data

    const buf = cpuSparklines.value[data.container_id] || []
    buf.push(data.cpu_percent)
    if (buf.length > SPARKLINE_BUFFER_SIZE) buf.shift()
    cpuSparklines.value[data.container_id] = buf
  }

  function onAlert(e: MessageEvent) {
    let data: ResourceAlert
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    alerts.value[data.container_id] = data
  }

  function onRecovery(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    delete alerts.value[data.container_id]
  }

  function onReconnected() {
    fetchSummary()
  }

  function connectSSE() {
    sseBus.on('resource.snapshot', onSnapshot)
    sseBus.on('resource.alert', onAlert)
    sseBus.on('resource.recovery', onRecovery)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('resource.snapshot', onSnapshot)
    sseBus.off('resource.alert', onAlert)
    sseBus.off('resource.recovery', onRecovery)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  async function fetchSummary() {
    try {
      summary.value = await getSummary(summaryQuery.value)
    } catch {
      // ignore
    }
  }

  // setFilter changes the global host filter and persists it. Reacting to the
  // change (refetching the entity lists + gauges) is wired in AppHeader, which
  // owns the dropdown and is always mounted — this keeps the store free of a
  // dashboard import cycle.
  function setFilter(value: string | null) {
    selected.value = value
    try {
      if (value) localStorage.setItem(FILTER_KEY, value)
      else localStorage.removeItem(FILTER_KEY)
    } catch {
      // ignore (private mode)
    }
  }

  // reconcile clears the selection when the selected agent is no longer enrolled
  // (revoked/deleted), falling back to "all resources".
  function reconcile(activeAgentIds: Set<string>) {
    const s = selected.value
    if (s && s !== 'local' && !activeAgentIds.has(s)) {
      setFilter(null)
    }
  }

  return {
    snapshots,
    alerts,
    summary,
    cpuSparklines,
    selected,
    entityQuery,
    summaryQuery,
    getSnapshot,
    getAlert,
    formattedSnapshot,
    formatBytes,
    formatPercent,
    connectSSE,
    disconnectSSE,
    fetchSummary,
    setFilter,
    reconcile,
  }
})
