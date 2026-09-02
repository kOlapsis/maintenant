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
  listEndpoints,
  type Endpoint,
  type ListEndpointsParams,
} from '@/services/endpointApi'
import { sseBus } from '@/services/sseBus'
import { useResourcesStore } from '@/stores/resources'
import { useAgentsStore } from '@/stores/agents'

export const useEndpointsStore = defineStore('endpoints', () => {
  const endpoints = ref<Endpoint[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected
  const configErrors = ref<{ container_name: string; label_key: string; error: string }[]>([])
  const totalCount = ref(0)

  // Filters
  const searchQuery = ref<string>('')
  const statusFilter = ref<string>('')
  const typeFilter = ref<string>('')
  const containerFilter = ref<string>('')
  // Retired endpoints are the ones whose container is gone. They are hidden by
  // default (a fleet with churn would otherwise accumulate dead cards), but
  // they have to be reachable, because they are the only ones you can delete
  // by hand.
  const showRetired = ref(false)

  const endpointsCount = computed(() => totalCount.value)

  // The reporting host is searchable too, so "web-02" narrows a fleet down to
  // one machine's endpoints.
  function agentText(ep: Endpoint): string {
    if (!ep.agent_id) return ''
    const agent = useAgentsStore().agents.find((a) => a.agent_id === ep.agent_id)
    return agent ? `${agent.label} ${agent.hostname}` : ''
  }

  function matchesSearch(ep: Endpoint, q: string): boolean {
    if (!q) return true
    return [ep.name, ep.target, ep.container_name, agentText(ep)].some((f) =>
      f?.toLowerCase().includes(q),
    )
  }

  // Everything except the status chips, so the chip counts say exactly how many
  // rows a chip would leave on screen.
  const searchAndSecondaryFiltered = computed(() => {
    const q = searchQuery.value.trim().toLowerCase()
    return endpoints.value.filter((e) => {
      if (typeFilter.value && e.endpoint_type !== typeFilter.value) return false
      if (containerFilter.value && e.container_name !== containerFilter.value) return false
      return matchesSearch(e, q)
    })
  })

  const filteredEndpoints = computed(() =>
    statusFilter.value
      ? searchAndSecondaryFiltered.value.filter((e) => e.status === statusFilter.value)
      : searchAndSecondaryFiltered.value,
  )

  const activeFilterCount = computed(
    () =>
      (typeFilter.value ? 1 : 0) +
      (containerFilter.value ? 1 : 0) +
      (showRetired.value ? 1 : 0),
  )

  function resetFilters() {
    const refetch = showRetired.value
    searchQuery.value = ''
    statusFilter.value = ''
    typeFilter.value = ''
    containerFilter.value = ''
    showRetired.value = false
    if (refetch) fetchEndpoints()
  }

  const endpointsByContainer = computed(() => {
    const map = new Map<string, Endpoint[]>()
    for (const ep of endpoints.value) {
      const list = map.get(ep.container_name) || []
      list.push(ep)
      map.set(ep.container_name, list)
    }
    return map
  })

  const statusCounts = computed(() => {
    const counts = { up: 0, down: 0, degraded: 0, unknown: 0 }
    for (const ep of searchAndSecondaryFiltered.value) {
      // A retired endpoint has no container behind it any more; its last known
      // status is history, not a state of the fleet. Once the operator asks to
      // see them, they count like the rest.
      if (!ep.active && !showRetired.value) continue
      if (ep.status in counts) {
        counts[ep.status as keyof typeof counts]++
      }
    }
    return counts
  })

  async function fetchEndpoints(params?: ListEndpointsParams) {
    const q = useResourcesStore().entityQuery
    let mergedParams = q ? { ...params, agent_id: q } : params
    if (showRetired.value) {
      mergedParams = { ...mergedParams, include_inactive: true }
    }
    loading.value = true
    error.value = null
    try {
      const res = await listEndpoints(mergedParams)
      endpoints.value = res.endpoints || []
      totalCount.value = res.total || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch endpoints'
    } finally {
      loading.value = false
    }
  }

  // applyEndpoint upserts a freshly fetched endpoint. An on-demand check only
  // emits an SSE event when the status actually changes, so the caller feeds the
  // result back here to keep the list in step without refetching everything.
  function applyEndpoint(ep: Endpoint) {
    const idx = endpoints.value.findIndex((e) => e.id === ep.id)
    if (idx >= 0) endpoints.value[idx] = ep
    else endpoints.value.push(ep)
  }

  function onDiscovered() {
    fetchEndpoints()
  }

  function onStatusChanged(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = endpoints.value.findIndex((ep) => ep.id === data.endpoint_id)
    if (idx >= 0) {
      endpoints.value[idx] = {
        ...endpoints.value[idx]!,
        status: data.new_status,
        last_response_time_ms: data.response_time_ms,
        last_http_status: data.http_status,
        last_error: data.error || undefined,
        last_check_at: data.timestamp,
      }
    } else {
      fetchEndpoints()
    }
  }

  function onRemoved(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    endpoints.value = endpoints.value.filter((ep) => ep.id !== data.endpoint_id)
  }

  function onAlert(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = endpoints.value.findIndex((ep) => ep.id === data.endpoint_id)
    if (idx >= 0) {
      endpoints.value[idx] = {
        ...endpoints.value[idx]!,
        alert_state: 'alerting',
        consecutive_failures: data.consecutive_failures,
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
    const idx = endpoints.value.findIndex((ep) => ep.id === data.endpoint_id)
    if (idx >= 0) {
      endpoints.value[idx] = {
        ...endpoints.value[idx]!,
        alert_state: 'normal',
        consecutive_successes: data.consecutive_successes,
      }
    }
  }

  function onConfigError(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    configErrors.value.push({
      container_name: data.container_name,
      label_key: data.label_key,
      error: data.error,
    })
    if (configErrors.value.length > 20) {
      configErrors.value = configErrors.value.slice(-20)
    }
  }

  function onReconnected() {
    fetchEndpoints()
  }

  function connectSSE() {
    sseBus.on('endpoint.discovered', onDiscovered)
    sseBus.on('endpoint.status_changed', onStatusChanged)
    sseBus.on('endpoint.removed', onRemoved)
    sseBus.on('endpoint.alert', onAlert)
    sseBus.on('endpoint.recovery', onRecovery)
    sseBus.on('endpoint.config_error', onConfigError)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('endpoint.discovered', onDiscovered)
    sseBus.off('endpoint.status_changed', onStatusChanged)
    sseBus.off('endpoint.removed', onRemoved)
    sseBus.off('endpoint.alert', onAlert)
    sseBus.off('endpoint.recovery', onRecovery)
    sseBus.off('endpoint.config_error', onConfigError)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  return {
    endpoints,
    endpointsCount,
    loading,
    error,
    sseConnected,
    configErrors,
    searchQuery,
    statusFilter,
    typeFilter,
    containerFilter,
    showRetired,
    filteredEndpoints,
    activeFilterCount,
    resetFilters,
    endpointsByContainer,
    statusCounts,
    fetchEndpoints,
    applyEndpoint,
    connectSSE,
    disconnectSSE,
  }
})
