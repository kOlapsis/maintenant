import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  listEndpoints,
  type Endpoint,
  type ListEndpointsParams,
} from '@/services/endpointApi'
import { sseBus } from '@/services/sseBus'

export const useEndpointsStore = defineStore('endpoints', () => {
  const endpoints = ref<Endpoint[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected
  const configErrors = ref<{ container_name: string; label_key: string; error: string }[]>([])
  const totalCount = ref(0)

  // Filters
  const statusFilter = ref<string>('')
  const typeFilter = ref<string>('')
  const containerFilter = ref<string>('')

  const endpointsCount = computed(() => totalCount.value)

  const filteredEndpoints = computed(() => {
    let result = endpoints.value
    if (statusFilter.value) {
      result = result.filter((e) => e.status === statusFilter.value)
    }
    if (typeFilter.value) {
      result = result.filter((e) => e.endpoint_type === typeFilter.value)
    }
    if (containerFilter.value) {
      result = result.filter((e) => e.container_name === containerFilter.value)
    }
    return result
  })

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
    const counts = { up: 0, down: 0, unknown: 0 }
    for (const ep of endpoints.value) {
      if (ep.status in counts) {
        counts[ep.status as keyof typeof counts]++
      }
    }
    return counts
  })

  async function fetchEndpoints(params?: ListEndpointsParams) {
    loading.value = true
    error.value = null
    try {
      const res = await listEndpoints(params)
      endpoints.value = res.endpoints || []
      totalCount.value = res.total || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch endpoints'
    } finally {
      loading.value = false
    }
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

  function connectSSE() {
    sseBus.on('endpoint.discovered', onDiscovered)
    sseBus.on('endpoint.status_changed', onStatusChanged)
    sseBus.on('endpoint.removed', onRemoved)
    sseBus.on('endpoint.alert', onAlert)
    sseBus.on('endpoint.recovery', onRecovery)
    sseBus.on('endpoint.config_error', onConfigError)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('endpoint.discovered', onDiscovered)
    sseBus.off('endpoint.status_changed', onStatusChanged)
    sseBus.off('endpoint.removed', onRemoved)
    sseBus.off('endpoint.alert', onAlert)
    sseBus.off('endpoint.recovery', onRecovery)
    sseBus.off('endpoint.config_error', onConfigError)
    sseBus.disconnect()
  }

  return {
    endpoints,
    endpointsCount,
    loading,
    error,
    sseConnected,
    configErrors,
    statusFilter,
    typeFilter,
    containerFilter,
    filteredEndpoints,
    endpointsByContainer,
    statusCounts,
    fetchEndpoints,
    connectSSE,
    disconnectSSE,
  }
})
