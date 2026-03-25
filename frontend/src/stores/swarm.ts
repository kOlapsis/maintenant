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
  fetchSwarmInfo,
  fetchSwarmNodes,
  type SwarmInfo,
  type SwarmNodeResponse,
} from '@/services/swarmApi'
import { sseBus } from '@/services/sseBus'

export interface CrashLoopState {
  service_id: string
  service_name: string
  failure_count: number
  last_error: string
  timestamp: string
}

export interface UpdateProgressState {
  service_id: string
  service_name: string
  state: string
  tasks_updated: number
  tasks_total: number
  old_image: string
  new_image: string
  message: string
  timestamp: string
}

export const useSwarmStore = defineStore('swarm', () => {
  const info = ref<SwarmInfo | null>(null)
  const nodes = ref<SwarmNodeResponse[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Enterprise state
  const crashLoops = ref<Map<string, CrashLoopState>>(new Map())
  const updateProgress = ref<Map<string, UpdateProgressState>>(new Map())

  const isActive = computed(() => info.value?.active === true)
  const isManager = computed(() => info.value?.is_manager === true)
  const managerCount = computed(() => {
    return nodes.value.filter((n) => n.role === 'manager').length
  })
  const workerCount = computed(() => {
    return nodes.value.filter((n) => n.role === 'worker').length
  })
  const readyCount = computed(() => {
    return nodes.value.filter((n) => n.status === 'ready').length
  })

  function isCrashLooping(serviceId: string): boolean {
    return crashLoops.value.has(serviceId)
  }

  function getUpdateProgress(serviceId: string): UpdateProgressState | undefined {
    return updateProgress.value.get(serviceId)
  }

  async function loadInfo() {
    try {
      info.value = await fetchSwarmInfo()
    } catch {
      info.value = { active: false }
    }
  }

  async function loadNodes() {
    loading.value = true
    error.value = null
    try {
      const resp = await fetchSwarmNodes()
      nodes.value = resp.nodes
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load nodes'
    } finally {
      loading.value = false
    }
  }

  function onSwarmStatus(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      info.value = {
        active: data.active,
        cluster_id: data.cluster_id,
        is_manager: data.is_manager,
        manager_count: data.manager_count,
        worker_count: data.worker_count,
      }
    } catch { /* ignore */ }
  }

  function onNodeStatusChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      const idx = nodes.value.findIndex((n) => n.node_id === data.node_id)
      if (idx >= 0) {
        const existing = nodes.value[idx]!
        nodes.value[idx] = {
          ...existing,
          status: (data.new_status ?? data.status ?? existing.status) as string,
          availability: (data.new_availability ?? data.availability ?? existing.availability) as string,
          last_seen_at: new Date().toISOString(),
          last_status_change_at: new Date().toISOString(),
        }
      } else {
        loadNodes()
      }
    } catch { /* ignore */ }
  }

  function onCrashLoopDetected(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      crashLoops.value.set(data.service_id, {
        service_id: data.service_id,
        service_name: data.service_name,
        failure_count: data.failure_count,
        last_error: data.last_error,
        timestamp: data.timestamp,
      })
    } catch { /* ignore */ }
  }

  function onCrashLoopRecovered(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      crashLoops.value.delete(data.service_id)
    } catch { /* ignore */ }
  }

  function onUpdateProgressEvent(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      updateProgress.value.set(data.service_id, {
        service_id: data.service_id,
        service_name: data.service_name,
        state: data.state,
        tasks_updated: data.tasks_updated,
        tasks_total: data.tasks_total,
        old_image: data.old_image,
        new_image: data.new_image,
        message: data.message,
        timestamp: data.timestamp,
      })
    } catch { /* ignore */ }
  }

  function onUpdateCompleted(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data)
      updateProgress.value.delete(data.service_id)
    } catch { /* ignore */ }
  }

  function startListening() {
    sseBus.on('swarm.status', onSwarmStatus)
    sseBus.on('swarm.node_status_changed', onNodeStatusChanged)
    sseBus.on('swarm.crash_loop_detected', onCrashLoopDetected)
    sseBus.on('swarm.crash_loop_recovered', onCrashLoopRecovered)
    sseBus.on('swarm.update_progress', onUpdateProgressEvent)
    sseBus.on('swarm.update_completed', onUpdateCompleted)
  }

  function stopListening() {
    sseBus.off('swarm.status', onSwarmStatus)
    sseBus.off('swarm.node_status_changed', onNodeStatusChanged)
    sseBus.off('swarm.crash_loop_detected', onCrashLoopDetected)
    sseBus.off('swarm.crash_loop_recovered', onCrashLoopRecovered)
    sseBus.off('swarm.update_progress', onUpdateProgressEvent)
    sseBus.off('swarm.update_completed', onUpdateCompleted)
  }

  return {
    info,
    nodes,
    loading,
    error,
    crashLoops,
    updateProgress,
    isActive,
    isManager,
    managerCount,
    workerCount,
    readyCount,
    isCrashLooping,
    getUpdateProgress,
    loadInfo,
    loadNodes,
    startListening,
    stopListening,
  }
})
