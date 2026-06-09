// Copyright 2026 Benjamin Touchard (kOlapsis)
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
  getPosture,
  getContainerPosture,
  listAcknowledgments,
  createAcknowledgment,
  deleteAcknowledgment,
  type InfrastructurePosture,
  type SecurityScore,
  type RiskAcknowledgment,
} from '@/services/postureApi'
import { sseBus } from '@/services/sseBus'

export const usePostureStore = defineStore('posture', () => {
  const posture = ref<InfrastructurePosture | null>(null)
  const loading = ref(false)
  const acknowledgments = ref<Record<string, RiskAcknowledgment[]>>({})

  function onPostureChanged() {
    fetchPosture()
  }

  function connectSSE() {
    sseBus.on('security.posture_changed', onPostureChanged)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('security.posture_changed', onPostureChanged)
    sseBus.disconnect()
  }

  async function fetchPosture() {
    loading.value = true
    try {
      posture.value = await getPosture()
    } catch {
      // ignore
    } finally {
      loading.value = false
    }
  }

  async function fetchContainerScore(containerId: string): Promise<SecurityScore | null> {
    try {
      return await getContainerPosture(containerId)
    } catch {
      return null
    }
  }

  async function fetchAcknowledgments(containerId?: string) {
    try {
      const data = await listAcknowledgments(containerId)
      if (containerId) {
        acknowledgments.value[containerId] = data.acknowledgments
      }
    } catch {
      // ignore
    }
  }

  async function acknowledgeRisk(body: {
    container_id: string
    finding_type: string
    finding_key: string
    acknowledged_by: string
    reason: string
  }) {
    await createAcknowledgment(body)
    await fetchPosture()
  }

  async function revokeAcknowledgment(id: string) {
    await deleteAcknowledgment(id)
    await fetchPosture()
  }

  return {
    posture,
    loading,
    acknowledgments,
    connectSSE,
    disconnectSSE,
    fetchPosture,
    fetchContainerScore,
    fetchAcknowledgments,
    acknowledgeRisk,
    revokeAcknowledgment,
  }
})
