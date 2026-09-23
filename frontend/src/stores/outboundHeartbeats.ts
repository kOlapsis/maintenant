// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  listOutboundHeartbeats,
  createOutboundHeartbeat,
  updateOutboundHeartbeat,
  deleteOutboundHeartbeat,
  sendOutboundHeartbeat,
  type OutboundHeartbeat,
  type OutboundHeartbeatInput,
} from '@/services/outboundHeartbeatApi'

export const useOutboundHeartbeatsStore = defineStore('outboundHeartbeats', () => {
  const outboundHeartbeats = ref<OutboundHeartbeat[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  function replace(hb: OutboundHeartbeat) {
    const idx = outboundHeartbeats.value.findIndex((h) => h.id === hb.id)
    if (idx >= 0) {
      outboundHeartbeats.value[idx] = hb
    } else {
      outboundHeartbeats.value.push(hb)
    }
  }

  async function fetchOutboundHeartbeats() {
    loading.value = true
    error.value = null
    try {
      const res = await listOutboundHeartbeats()
      outboundHeartbeats.value = res.outbound_heartbeats || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch outgoing targets'
    } finally {
      loading.value = false
    }
  }

  async function create(data: OutboundHeartbeatInput): Promise<OutboundHeartbeat> {
    const created = await createOutboundHeartbeat(data)
    outboundHeartbeats.value.push(created)
    return created
  }

  async function update(id: string, data: OutboundHeartbeatInput): Promise<OutboundHeartbeat> {
    const updated = await updateOutboundHeartbeat(id, data)
    replace(updated)
    return updated
  }

  async function remove(id: string): Promise<void> {
    await deleteOutboundHeartbeat(id)
    outboundHeartbeats.value = outboundHeartbeats.value.filter((hb) => hb.id !== id)
  }

  async function sendNow(id: string): Promise<OutboundHeartbeat> {
    const updated = await sendOutboundHeartbeat(id)
    replace(updated)
    return updated
  }

  return {
    outboundHeartbeats,
    loading,
    error,
    fetchOutboundHeartbeats,
    create,
    update,
    remove,
    sendNow,
  }
})
