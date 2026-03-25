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
  listContainers,
  type Container,
  type ListContainersParams,
} from '@/services/containerApi'
import { sseBus } from '@/services/sseBus'

export interface ContainerGroup {
  name: string
  source: string
  containers: Container[]
}

export const useContainersStore = defineStore('containers', () => {
  const groups = ref<ContainerGroup[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sseConnected = sseBus.connected
  const runtimeConnected = ref(true)
  const runtimeName = ref('docker')
  const runtimeLabel = ref('Docker')
  const totalCount = ref(0)
  const archivedCount = ref(0)

  const expandedControllers = ref<Set<string>>(new Set())

  const allContainers = computed(() =>
    groups.value.flatMap((g) => g.containers),
  )

  const activeContainers = computed(() =>
    allContainers.value.filter((c) => !c.archived),
  )

  const containerCount = computed(() => activeContainers.value.length)

  const isKubernetesMode = computed(() => runtimeName.value === 'kubernetes')
  const isSwarmMode = ref(false)

  function toggleController(key: string) {
    if (expandedControllers.value.has(key)) {
      expandedControllers.value.delete(key)
    } else {
      expandedControllers.value.add(key)
    }
  }

  function isControllerExpanded(key: string): boolean {
    return expandedControllers.value.has(key)
  }

  function findContainerIndex(id: number): { groupIdx: number; containerIdx: number } | null {
    for (let gi = 0; gi < groups.value.length; gi++) {
      const ci = groups.value[gi]!.containers.findIndex((c) => c.id === id)
      if (ci >= 0) return { groupIdx: gi, containerIdx: ci }
    }
    return null
  }

  async function fetchContainers(params?: ListContainersParams) {
    loading.value = true
    error.value = null
    try {
      const res = await listContainers(params)
      groups.value = res.groups || []
      totalCount.value = res.total || 0
      archivedCount.value = res.archived_count || 0
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch containers'
    } finally {
      loading.value = false
    }
  }

  function onDiscovered() {
    fetchContainers()
  }

  function onStateChanged(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const pos = findContainerIndex(data.id)
    if (pos) {
      const c = groups.value[pos.groupIdx]!.containers[pos.containerIdx]!
      groups.value[pos.groupIdx]!.containers[pos.containerIdx] = {
        ...c,
        state: data.state,
        health_status: data.health_status,
        last_state_change_at: data.timestamp,
      }
    }
  }

  function onHealthChanged(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const pos = findContainerIndex(data.id)
    if (pos) {
      const c = groups.value[pos.groupIdx]!.containers[pos.containerIdx]!
      groups.value[pos.groupIdx]!.containers[pos.containerIdx] = {
        ...c,
        health_status: data.health_status,
        last_state_change_at: data.timestamp,
      }
    }
  }

  function onArchived(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const pos = findContainerIndex(data.id)
    if (pos) {
      const c = groups.value[pos.groupIdx]!.containers[pos.containerIdx]!
      groups.value[pos.groupIdx]!.containers[pos.containerIdx] = {
        ...c,
        archived: true,
        archived_at: data.archived_at,
      }
    }
  }

  function onRuntimeStatus(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    if (typeof data.connected === 'boolean') runtimeConnected.value = data.connected
    if (data.name) runtimeName.value = data.name
    if (data.label) runtimeLabel.value = data.label
  }

  function onSwarmServiceEvent() {
    fetchContainers()
  }

  function onSwarmStatus(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    if (typeof data.active === 'boolean') isSwarmMode.value = data.active
  }

  function onReconnected() {
    fetchContainers()
  }

  function connectSSE() {
    sseBus.on('container.discovered', onDiscovered)
    sseBus.on('container.state_changed', onStateChanged)
    sseBus.on('container.health_changed', onHealthChanged)
    sseBus.on('container.archived', onArchived)
    sseBus.on('runtime.status', onRuntimeStatus)
    sseBus.on('swarm.service_discovered', onSwarmServiceEvent)
    sseBus.on('swarm.service_updated', onSwarmServiceEvent)
    sseBus.on('swarm.service_removed', onSwarmServiceEvent)
    sseBus.on('swarm.status', onSwarmStatus)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('container.discovered', onDiscovered)
    sseBus.off('container.state_changed', onStateChanged)
    sseBus.off('container.health_changed', onHealthChanged)
    sseBus.off('container.archived', onArchived)
    sseBus.off('runtime.status', onRuntimeStatus)
    sseBus.off('swarm.service_discovered', onSwarmServiceEvent)
    sseBus.off('swarm.service_updated', onSwarmServiceEvent)
    sseBus.off('swarm.service_removed', onSwarmServiceEvent)
    sseBus.off('swarm.status', onSwarmStatus)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  return {
    groups,
    loading,
    error,
    sseConnected,
    runtimeConnected,
    runtimeName,
    runtimeLabel,
    isKubernetesMode,
    isSwarmMode,
    allContainers,
    activeContainers,
    containerCount,
    totalCount,
    archivedCount,
    expandedControllers,
    toggleController,
    isControllerExpanded,
    fetchContainers,
    connectSSE,
    disconnectSSE,
  }
})
