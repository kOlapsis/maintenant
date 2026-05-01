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

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface Agent {
  agent_id: string
  hostname: string
  label: string
  os_arch: string
  agent_version: string
  detected_runtime: 'docker' | 'swarm' | 'kubernetes'
  status: 'active' | 'revoked'
  connection_state?: 'connected' | 'disconnected'
  last_seen_at: string | null
  created_at: string
  revoked_at: string | null
  revoked_by: string | null
}

export interface EnrollmentTokenMasked {
  token_id: string
  token_masked: string
  created_by: string
  created_at: string
  expires_at: string
  consumed_at: string | null
  consumed_by_agent_id: string | null
}

export interface EnrollmentTokenCreated extends EnrollmentTokenMasked {
  token: string
  install_command: string
  warnings?: string[]
}

export interface AgentMetrics {
  total: number
  by_status: { active: number; revoked: number }
  by_runtime: { docker: number; swarm: number; kubernetes: number }
  by_connection_state: { connected: number; disconnected: number }
  total_events_per_second_observed_5m: number
}

export function listAgents(params?: {
  status?: string
  connection_state?: string
}): Promise<{ agents: Agent[] }> {
  const url = new URL(`${API_BASE}/agents`, window.location.origin)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.connection_state) url.searchParams.set('connection_state', params.connection_state)
  return apiFetch<{ agents: Agent[] }>(url.toString())
}

export function getAgent(id: string): Promise<Agent> {
  return apiFetch<Agent>(`${API_BASE}/agents/${id}`)
}

export function updateAgentLabel(id: string, label: string): Promise<Agent> {
  return apiFetch<Agent>(`${API_BASE}/agents/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ label }),
  })
}

export function revokeAgent(id: string): Promise<Agent> {
  return apiFetch<Agent>(`${API_BASE}/agents/${id}/revoke`, { method: 'POST' })
}

export function deleteAgent(id: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/agents/${id}`, { method: 'DELETE' })
}

export function listEnrollmentTokens(params?: {
  include_expired?: boolean
  include_consumed?: boolean
}): Promise<{ tokens: EnrollmentTokenMasked[] }> {
  const url = new URL(`${API_BASE}/agents/enrollment-tokens`, window.location.origin)
  if (params?.include_expired !== undefined)
    url.searchParams.set('include_expired', String(params.include_expired))
  if (params?.include_consumed !== undefined)
    url.searchParams.set('include_consumed', String(params.include_consumed))
  return apiFetch<{ tokens: EnrollmentTokenMasked[] }>(url.toString())
}

export function createEnrollmentToken(params?: {
  ttl_hours?: number
}): Promise<EnrollmentTokenCreated> {
  return apiFetch<EnrollmentTokenCreated>(`${API_BASE}/agents/enrollment-tokens`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params ?? {}),
  })
}

export function getEnrollmentToken(tokenId: string): Promise<EnrollmentTokenMasked> {
  return apiFetch<EnrollmentTokenMasked>(`${API_BASE}/agents/enrollment-tokens/${tokenId}`)
}

export function deleteEnrollmentToken(tokenId: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/agents/enrollment-tokens/${tokenId}`, { method: 'DELETE' })
}

export function getAgentMetrics(): Promise<AgentMetrics> {
  return apiFetch<AgentMetrics>(`${API_BASE}/agents/metrics`)
}
