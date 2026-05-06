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
import { useEscalationApi } from '@/composables/useEscalationApi'
import type { EscalationPolicy, EscalationLimits, PolicyRequest } from '@/types/escalation'

export const useEscalationStore = defineStore('escalation', () => {
  const policies = ref<EscalationPolicy[]>([])
  const limits = ref<EscalationLimits | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const api = useEscalationApi()

  async function fetchPolicies() {
    loading.value = true
    error.value = null
    try {
      const res = await api.listPolicies()
      policies.value = res.policies
      limits.value = res.limits
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch escalation policies'
    } finally {
      loading.value = false
    }
  }

  async function createPolicy(req: PolicyRequest) {
    const created = await api.createPolicy(req)
    policies.value = [created, ...policies.value]
    if (limits.value && created.active) {
      limits.value = { ...limits.value, current_active: limits.value.current_active + 1 }
    }
    return created
  }

  async function updatePolicy(id: number, req: PolicyRequest) {
    const updated = await api.updatePolicy(id, req)
    const idx = policies.value.findIndex((p) => p.id === id)
    const existing = idx !== -1 ? policies.value[idx] : undefined
    if (existing !== undefined) {
      const wasActive = existing.active
      policies.value[idx] = updated
      if (limits.value) {
        if (!wasActive && updated.active) {
          limits.value = { ...limits.value, current_active: limits.value.current_active + 1 }
        } else if (wasActive && !updated.active) {
          limits.value = { ...limits.value, current_active: Math.max(0, limits.value.current_active - 1) }
        }
      }
    }
    return updated
  }

  async function setPolicyActive(id: number, active: boolean) {
    await api.setPolicyActive(id, active)
    const idx = policies.value.findIndex((p) => p.id === id)
    const existing = idx !== -1 ? policies.value[idx] : undefined
    if (existing !== undefined) {
      const wasActive = existing.active
      policies.value[idx] = { ...existing, active }
      if (limits.value) {
        if (!wasActive && active) {
          limits.value = { ...limits.value, current_active: limits.value.current_active + 1 }
        } else if (wasActive && !active) {
          limits.value = { ...limits.value, current_active: Math.max(0, limits.value.current_active - 1) }
        }
      }
    }
  }

  async function deletePolicy(id: number) {
    await api.deletePolicy(id)
    const removed = policies.value.find((p) => p.id === id)
    policies.value = policies.value.filter((p) => p.id !== id)
    if (limits.value && removed?.active) {
      limits.value = {
        ...limits.value,
        current_active: Math.max(0, limits.value.current_active - 1),
      }
    }
  }

  return {
    policies,
    limits,
    loading,
    error,
    fetchPolicies,
    createPolicy,
    updatePolicy,
    setPolicyActive,
    deletePolicy,
  }
})
