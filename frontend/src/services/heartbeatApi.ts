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

export interface Heartbeat {
  id: string
  uuid: string
  name: string
  status: 'new' | 'up' | 'down' | 'started' | 'paused'
  alert_state: 'normal' | 'alerting'
  interval_seconds: number
  grace_seconds: number
  last_ping_at?: string
  next_deadline_at?: string
  current_run_started_at?: string
  last_exit_code?: number
  last_duration_ms?: number
  consecutive_failures: number
  consecutive_successes: number
  active: boolean
  created_at: string
  updated_at: string
  agent_id?: string | null
}

export interface HeartbeatPing {
  id: string
  heartbeat_id: string
  ping_type: 'success' | 'start' | 'exit_code'
  exit_code?: number
  source_ip: string
  http_method: string
  payload?: string
  timestamp: string
}

export interface HeartbeatExecution {
  id: string
  heartbeat_id: string
  started_at?: string
  completed_at?: string
  duration_ms?: number
  exit_code?: number
  outcome: 'success' | 'failure' | 'timeout' | 'in_progress'
  payload?: string
}

export interface CreateHeartbeatInput {
  name: string
  interval_seconds: number
  grace_seconds: number
}

export interface UpdateHeartbeatInput {
  name?: string
  interval_seconds?: number
  grace_seconds?: number
}

export interface HeartbeatsResponse {
  heartbeats: Heartbeat[]
  total: number
}

export interface HeartbeatDetailResponse {
  heartbeat: Heartbeat
  snippets?: Record<string, string>
}

export interface ExecutionsResponse {
  executions: HeartbeatExecution[]
  total: number
}

export interface PingsResponse {
  pings: HeartbeatPing[]
  total: number
}

function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  return apiFetch<T>(url, init)
}

export function listHeartbeats(agentId?: string): Promise<HeartbeatsResponse> {
  const url = new URL(`${API_BASE}/heartbeats`, window.location.origin)
  if (agentId) url.searchParams.set('agent_id', agentId)
  return fetchJSON<HeartbeatsResponse>(url.toString())
}

export function getHeartbeat(id: string): Promise<HeartbeatDetailResponse> {
  return fetchJSON<HeartbeatDetailResponse>(`${API_BASE}/heartbeats/${id}`)
}

export function createHeartbeat(data: CreateHeartbeatInput): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateHeartbeat(id: string, data: UpdateHeartbeatInput): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteHeartbeat(id: string): Promise<void> {
  return fetchJSON<void>(`${API_BASE}/heartbeats/${id}`, { method: 'DELETE' })
}

export function pauseHeartbeat(id: string): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}/pause`, { method: 'POST' })
}

export function resumeHeartbeat(id: string): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}/resume`, { method: 'POST' })
}

export function listExecutions(id: string, params?: { limit?: number; offset?: number }): Promise<ExecutionsResponse> {
  const url = new URL(`${API_BASE}/heartbeats/${id}/executions`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<ExecutionsResponse>(url.toString())
}

export function listPings(id: string, params?: { limit?: number; offset?: number }): Promise<PingsResponse> {
  const url = new URL(`${API_BASE}/heartbeats/${id}/pings`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<PingsResponse>(url.toString())
}
