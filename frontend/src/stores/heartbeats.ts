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
  listHeartbeats,
  type Heartbeat,
} from '@/services/heartbeatApi'
import { sseBus } from '@/services/sseBus'

export const useHeartbeatsStore = defineStore('heartbeats', () => {
  const heartbeats = ref<Heartbeat[]>([])
  const totalCount = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected
  const agentFilter = ref<string | null>(null)

  const heartbeatsCount = computed(() => totalCount.value)

  const statusCounts = computed(() => {
    const counts = { new: 0, up: 0, down: 0, started: 0, paused: 0 }
    for (const hb of heartbeats.value) {
      if (hb.status in counts) {
        counts[hb.status as keyof typeof counts]++
      }
    }
    return counts
  })

  async function fetchHeartbeats(filter?: string | null) {
    if (filter !== undefined) agentFilter.value = filter
    loading.value = true
    error.value = null
    try {
      const res = await listHeartbeats(agentFilter.value ?? undefined)
      heartbeats.value = res.heartbeats || []
      totalCount.value = res.total || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch heartbeats'
    } finally {
      loading.value = false
    }
  }

  function onCreated() {
    fetchHeartbeats()
  }

  function onPingReceived(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = heartbeats.value.findIndex((hb) => hb.id === data.heartbeat_id)
    if (idx >= 0) {
      heartbeats.value[idx] = {
        ...heartbeats.value[idx]!,
        status: data.status,
      }
    } else {
      fetchHeartbeats()
    }
  }

  function onStatusChanged(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = heartbeats.value.findIndex((hb) => hb.id === data.heartbeat_id)
    if (idx >= 0) {
      heartbeats.value[idx] = {
        ...heartbeats.value[idx]!,
        status: data.new_status,
      }
    } else {
      fetchHeartbeats()
    }
  }

  function onAlert(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = heartbeats.value.findIndex((hb) => hb.id === data.heartbeat_id)
    if (idx >= 0) {
      heartbeats.value[idx] = {
        ...heartbeats.value[idx]!,
        alert_state: 'alerting',
      }
    }
  }

  function onRecovery(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = heartbeats.value.findIndex((hb) => hb.id === data.heartbeat_id)
    if (idx >= 0) {
      heartbeats.value[idx] = {
        ...heartbeats.value[idx]!,
        alert_state: 'normal',
      }
    }
  }

  function onDeleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    heartbeats.value = heartbeats.value.filter((hb) => hb.id !== data.heartbeat_id)
  }

  function onReconnected() {
    fetchHeartbeats()
  }

  function connectSSE() {
    sseBus.on('heartbeat.created', onCreated)
    sseBus.on('heartbeat.ping_received', onPingReceived)
    sseBus.on('heartbeat.status_changed', onStatusChanged)
    sseBus.on('heartbeat.alert', onAlert)
    sseBus.on('heartbeat.recovery', onRecovery)
    sseBus.on('heartbeat.deleted', onDeleted)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('heartbeat.created', onCreated)
    sseBus.off('heartbeat.ping_received', onPingReceived)
    sseBus.off('heartbeat.status_changed', onStatusChanged)
    sseBus.off('heartbeat.alert', onAlert)
    sseBus.off('heartbeat.recovery', onRecovery)
    sseBus.off('heartbeat.deleted', onDeleted)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  return {
    heartbeats,
    heartbeatsCount,
    loading,
    error,
    sseConnected,
    agentFilter,
    statusCounts,
    fetchHeartbeats,
    connectSSE,
    disconnectSSE,
  }
})
