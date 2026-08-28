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

import { apiFetch, apiFetchVoid } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface CategoryScore {
  name: string
  weight: number
  sub_score: number
  applicable: boolean
  issue_count: number
  summary: string
}

export interface ContainerRisk {
  container_id: string
  container_name: string
  score: number
  color: string
  top_issue: string
}

export interface CategorySummary {
  name: string
  total_issues: number
  summary: string
}

export interface InfrastructurePosture {
  score: number
  color: string
  container_count: number
  scored_count: number
  is_partial: boolean
  categories: CategorySummary[]
  top_risks: ContainerRisk[]
  computed_at: string
}

export interface SecurityScore {
  container_id: string
  container_name: string
  score: number
  color: string
  categories: CategoryScore[]
  applicable_count: number
  computed_at: string
  is_partial: boolean
}

export interface ContainerPostureList {
  containers: SecurityScore[]
  total: number
  limit: number
  offset: number
}

export interface RiskAcknowledgment {
  id: string
  container_external_id: string
  finding_type: string
  finding_key: string
  acknowledged_by: string
  reason: string
  acknowledged_at: string
}

export interface AcknowledgmentList {
  acknowledgments: RiskAcknowledgment[]
}

export function getPosture(): Promise<InfrastructurePosture> {
  return apiFetch(`${API_BASE}/security/posture`)
}

export function getContainerPosture(id: string): Promise<SecurityScore> {
  return apiFetch(`${API_BASE}/security/posture/containers/${id}`)
}

export function listContainerPostures(limit = 50, offset = 0): Promise<ContainerPostureList> {
  return apiFetch(`${API_BASE}/security/posture/containers?limit=${limit}&offset=${offset}`)
}

export function createAcknowledgment(body: {
  container_id: string
  finding_type: string
  finding_key: string
  acknowledged_by: string
  reason: string
}): Promise<RiskAcknowledgment> {
  return apiFetch(`${API_BASE}/security/acknowledgments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function deleteAcknowledgment(id: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/security/acknowledgments/${id}`, {
    method: 'DELETE',
  })
}

export function listAcknowledgments(containerId?: string): Promise<AcknowledgmentList> {
  const params = containerId ? `?container_id=${containerId}` : ''
  return apiFetch(`${API_BASE}/security/acknowledgments${params}`)
}
