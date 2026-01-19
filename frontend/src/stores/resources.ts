import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  getSummary,
  type ResourceSnapshot,
  type ResourceSummary,
} from '@/services/resourceApi'
import { sseBus } from '@/services/sseBus'

export interface ResourceAlert {
  container_id: number
  container_name: string
  alert_type: string
  current_value: number
  threshold: number
  timestamp: string
}

const SPARKLINE_BUFFER_SIZE = 20

export const useResourcesStore = defineStore('resources', () => {
  const snapshots = ref<Record<number, ResourceSnapshot>>({})
  const alerts = ref<Record<number, ResourceAlert>>({})
  const summary = ref<ResourceSummary | null>(null)
  const cpuSparklines = ref<Record<number, number[]>>({})

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
    return (containerId: number) => snapshots.value[containerId] || null
  })

  const getAlert = computed(() => {
    return (containerId: number) => alerts.value[containerId] || null
  })

  const formattedSnapshot = computed(() => {
    return (containerId: number) => {
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

  function connectSSE() {
    sseBus.on('resource.snapshot', onSnapshot)
    sseBus.on('resource.alert', onAlert)
    sseBus.on('resource.recovery', onRecovery)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('resource.snapshot', onSnapshot)
    sseBus.off('resource.alert', onAlert)
    sseBus.off('resource.recovery', onRecovery)
    sseBus.disconnect()
  }

  async function fetchSummary() {
    try {
      summary.value = await getSummary()
    } catch {
      // ignore
    }
  }

  return {
    snapshots,
    alerts,
    summary,
    cpuSparklines,
    getSnapshot,
    getAlert,
    formattedSnapshot,
    formatBytes,
    formatPercent,
    connectSSE,
    disconnectSSE,
    fetchSummary,
  }
})
