// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  fetchInsights,
  fetchContainerInsights,
  fetchSecuritySummary,
  type ContainerInsights,
  type InsightSummary,
} from '@/services/securityApi'
import { sseBus } from '@/services/sseBus'

export const useSecurityStore = defineStore('security', () => {
  const insightsByContainer = ref<Record<number, ContainerInsights>>({})
  const summary = ref<InsightSummary | null>(null)
  const loading = ref(false)

  const totalAffected = computed(() => summary.value?.total_containers_affected ?? 0)
  const totalInsights = computed(() => summary.value?.total_insights ?? 0)

  function onInsightsChanged() {
    fetchAll()
  }

  function onInsightsResolved() {
    fetchAll()
  }

  function connectSSE() {
    sseBus.on('security.insights_changed', onInsightsChanged)
    sseBus.on('security.insights_resolved', onInsightsResolved)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('security.insights_changed', onInsightsChanged)
    sseBus.off('security.insights_resolved', onInsightsResolved)
    sseBus.disconnect()
  }

  async function fetchAll() {
    loading.value = true
    try {
      const data = await fetchInsights()
      const map: Record<number, ContainerInsights> = {}
      for (const ci of data.containers) {
        map[ci.container_id] = ci
      }
      insightsByContainer.value = map
      summary.value = data.summary
    } catch {
      // ignore
    } finally {
      loading.value = false
    }
  }

  async function fetchForContainer(containerId: number) {
    try {
      const data = await fetchContainerInsights(containerId)
      insightsByContainer.value[containerId] = data
    } catch {
      // ignore
    }
  }

  async function fetchSummary() {
    try {
      summary.value = await fetchSecuritySummary()
    } catch {
      // ignore
    }
  }

  function getContainerInsights(containerId: number): ContainerInsights | null {
    return insightsByContainer.value[containerId] ?? null
  }

  return {
    insightsByContainer,
    summary,
    loading,
    totalAffected,
    totalInsights,
    connectSSE,
    disconnectSSE,
    fetchAll,
    fetchForContainer,
    fetchSummary,
    getContainerInsights,
  }
})
