// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

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
  const insightsByContainer = ref<Record<string, ContainerInsights>>({})
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
      const map: Record<string, ContainerInsights> = {}
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

  async function fetchForContainer(containerId: string) {
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

  function getContainerInsights(containerId: string): ContainerInsights | null {
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
