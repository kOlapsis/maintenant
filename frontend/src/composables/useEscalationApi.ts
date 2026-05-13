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

import { apiFetch, apiFetchVoid } from '@/services/apiFetch'
import type {
  EscalationPolicy,
  EscalationLimits,
  EscalationRun,
  EscalationDelivery,
  PolicyRequest,
  OverlapWarning,
} from '@/types/escalation'

const API_BASE = '/api/v1'

export function useEscalationApi() {
  function listPolicies(
    activeOnly?: boolean,
  ): Promise<{ policies: EscalationPolicy[]; limits: EscalationLimits }> {
    const url = new URL(`${API_BASE}/escalation-policies`, window.location.origin)
    if (activeOnly) url.searchParams.set('active', 'true')
    return apiFetch(url.toString())
  }

  function getPolicy(id: number): Promise<EscalationPolicy> {
    return apiFetch(`${API_BASE}/escalation-policies/${id}`)
  }

  function createPolicy(req: PolicyRequest): Promise<EscalationPolicy> {
    return apiFetch(`${API_BASE}/escalation-policies`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    })
  }

  function updatePolicy(id: number, req: PolicyRequest): Promise<EscalationPolicy> {
    return apiFetch(`${API_BASE}/escalation-policies/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    })
  }

  function setPolicyActive(
    id: number,
    active: boolean,
  ): Promise<{ id: number; active: boolean; updated_at: string }> {
    return apiFetch(`${API_BASE}/escalation-policies/${id}/active`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ active }),
    })
  }

  function deletePolicy(id: number): Promise<void> {
    return apiFetchVoid(`${API_BASE}/escalation-policies/${id}`, { method: 'DELETE' })
  }

  function listRunsForAlert(alertId: number): Promise<{ runs: EscalationRun[] }> {
    return apiFetch(`${API_BASE}/alerts/${alertId}/escalation-runs`)
  }

  function getEscalationRun(
    id: number,
  ): Promise<EscalationRun & { deliveries: EscalationDelivery[] }> {
    return apiFetch(`${API_BASE}/escalation-runs/${id}`)
  }

  function overlapProbe(req: PolicyRequest): Promise<{ overlapping: OverlapWarning[] }> {
    return apiFetch(`${API_BASE}/escalation-policies/overlap-probe`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    })
  }

  return {
    listPolicies,
    getPolicy,
    createPolicy,
    updatePolicy,
    setPolicyActive,
    deletePolicy,
    listRunsForAlert,
    getEscalationRun,
    overlapProbe,
  }
}
