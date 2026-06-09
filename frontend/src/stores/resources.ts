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
  getResourceHosts,
  type ResourceSnapshot,
  type ResourceSummary,
  type ResourceHost,
} from '@/services/resourceApi'
import { sseBus } from '@/services/sseBus'
import { isLocalAgent } from '@/services/apiFetch'

// Persisted selected host for the resources view ('' === local server).
const SELECTED_HOST_KEY = 'pb:resources:selected-host'

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

  // Multi-host: list of selectable hosts and the current selection ('' = local).
  const hosts = ref<ResourceHost[]>([])
  const selectedHostId = ref<string>(localStorage.getItem(SELECTED_HOST_KEY) ?? '')

  // The selector is only meaningful when more than one host exists.
  const multiHost = computed(() => hosts.value.length > 1)

  // Selection key for a host: '' for the local server (the backend now sends the
  // sentinel agent id for local entities; older builds sent ''), else the agent id.
  function hostKey(h: ResourceHost): string {
    return h.is_local || isLocalAgent(h.agent_id) ? '' : h.agent_id
  }

  // Query value to scope API calls: undefined when there is a single host
  // (preserve prior behaviour), else 'local' or the agent id.
  const hostQuery = computed<string | undefined>(() => {
    if (!multiHost.value) return undefined
    return selectedHostId.value === '' ? 'local' : selectedHostId.value
  })

  const selectedHost = computed(() => hosts.value.find((h) => hostKey(h) === selectedHostId.value) ?? null)

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
      summary.value = await getSummary(hostQuery.value)
    } catch {
      // ignore
    }
  }

  async function fetchHosts() {
    try {
      const res = await getResourceHosts()
      hosts.value = res.hosts || []
      // Reset to local if the previously selected host disappeared.
      if (selectedHostId.value && !hosts.value.some((h) => hostKey(h) === selectedHostId.value)) {
        selectHost('')
      }
    } catch {
      // ignore
    }
  }

  function selectHost(agentId: string) {
    selectedHostId.value = agentId
    try {
      localStorage.setItem(SELECTED_HOST_KEY, agentId)
    } catch {
      // ignore (private mode)
    }
    fetchSummary()
  }

  return {
    snapshots,
    alerts,
    summary,
    cpuSparklines,
    hosts,
    selectedHostId,
    selectedHost,
    multiHost,
    hostKey,
    hostQuery,
    getSnapshot,
    getAlert,
    formattedSnapshot,
    formatBytes,
    formatPercent,
    connectSSE,
    disconnectSSE,
    fetchSummary,
    fetchHosts,
    selectHost,
  }
})
