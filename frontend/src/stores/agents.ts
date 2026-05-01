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
import { ref } from 'vue'
import {
  listAgents,
  updateAgentLabel as apiUpdateAgentLabel,
  revokeAgent as apiRevokeAgent,
  deleteAgent as apiDeleteAgent,
  listEnrollmentTokens,
  createEnrollmentToken,
  deleteEnrollmentToken,
  getAgentMetrics,
  type Agent,
  type AgentMetrics,
  type EnrollmentTokenCreated,
  type EnrollmentTokenMasked,
} from '@/services/agentApi'
import { sseBus } from '@/services/sseBus'

export const useAgentsStore = defineStore('agents', () => {
  const agents = ref<Agent[]>([])
  const tokens = ref<EnrollmentTokenMasked[]>([])
  const metrics = ref<AgentMetrics | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected

  async function fetchAgents() {
    loading.value = true
    error.value = null
    try {
      const res = await listAgents()
      agents.value = res.agents || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch agents'
    } finally {
      loading.value = false
    }
  }

  async function fetchTokens() {
    try {
      const res = await listEnrollmentTokens()
      tokens.value = res.tokens || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch tokens'
    }
  }

  async function fetchMetrics() {
    try {
      metrics.value = await getAgentMetrics()
    } catch {
      // metrics are optional — leave previous value
    }
  }

  async function createToken(ttlHours?: number): Promise<EnrollmentTokenCreated> {
    const result = await createEnrollmentToken({ ttl_hours: ttlHours })
    // Refresh the masked token list but do NOT persist the cleartext result
    await fetchTokens()
    return result
  }

  async function updateAgentLabel(id: string, label: string): Promise<void> {
    const updated = await apiUpdateAgentLabel(id, label)
    const idx = agents.value.findIndex((a) => a.agent_id === id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, label: updated.label }
    }
  }

  async function revokeAgent(id: string) {
    await apiRevokeAgent(id)
    const idx = agents.value.findIndex((a) => a.agent_id === id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, status: 'revoked' }
    }
  }

  async function deleteAgent(id: string) {
    await apiDeleteAgent(id)
    agents.value = agents.value.filter((a) => a.agent_id !== id)
  }

  async function deleteToken(tokenId: string) {
    await deleteEnrollmentToken(tokenId)
    tokens.value = tokens.value.filter((t) => t.token_id !== tokenId)
  }

  function onAgentCreated() {
    fetchAgents()
    fetchMetrics()
  }

  function onAgentUpdated(e: MessageEvent) {
    let data: { agent_id?: string; label?: string }
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = agents.value.findIndex((a) => a.agent_id === data.agent_id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, ...data }
    } else {
      fetchAgents()
    }
  }

  function onAgentRevoked(e: MessageEvent) {
    let data: { agent_id?: string }
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = agents.value.findIndex((a) => a.agent_id === data.agent_id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, status: 'revoked' }
    }
    fetchMetrics()
  }

  function onAgentDeleted(e: MessageEvent) {
    let data: { agent_id?: string }
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    agents.value = agents.value.filter((a) => a.agent_id !== data.agent_id)
    fetchMetrics()
  }

  function onAgentConnected(e: MessageEvent) {
    let data: { agent_id?: string }
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = agents.value.findIndex((a) => a.agent_id === data.agent_id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, connection_state: 'connected' }
    }
    fetchMetrics()
  }

  function onAgentDisconnected(e: MessageEvent) {
    let data: { agent_id?: string }
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = agents.value.findIndex((a) => a.agent_id === data.agent_id)
    if (idx >= 0) {
      agents.value[idx] = { ...agents.value[idx]!, connection_state: 'disconnected' }
    }
    fetchMetrics()
  }

  function onReconnected() {
    fetchAgents()
    fetchMetrics()
  }

  function connectSSE() {
    sseBus.on('agent.created', onAgentCreated)
    sseBus.on('agent.updated', onAgentUpdated)
    sseBus.on('agent.revoked', onAgentRevoked)
    sseBus.on('agent.deleted', onAgentDeleted)
    sseBus.on('agent.connected', onAgentConnected)
    sseBus.on('agent.disconnected', onAgentDisconnected)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('agent.created', onAgentCreated)
    sseBus.off('agent.updated', onAgentUpdated)
    sseBus.off('agent.revoked', onAgentRevoked)
    sseBus.off('agent.deleted', onAgentDeleted)
    sseBus.off('agent.connected', onAgentConnected)
    sseBus.off('agent.disconnected', onAgentDisconnected)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  return {
    agents,
    tokens,
    metrics,
    loading,
    error,
    sseConnected,
    fetchAgents,
    fetchTokens,
    fetchMetrics,
    createToken,
    updateAgentLabel,
    revokeAgent,
    deleteAgent,
    deleteToken,
    connectSSE,
    disconnectSSE,
  }
})
