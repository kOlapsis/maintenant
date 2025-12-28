const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface Heartbeat {
  id: number
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
}

export interface HeartbeatPing {
  id: number
  heartbeat_id: number
  ping_type: 'success' | 'start' | 'exit_code'
  exit_code?: number
  source_ip: string
  http_method: string
  payload?: string
  timestamp: string
}

export interface HeartbeatExecution {
  id: number
  heartbeat_id: number
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

export function listHeartbeats(): Promise<HeartbeatsResponse> {
  return fetchJSON<HeartbeatsResponse>(`${API_BASE}/heartbeats`)
}

export function getHeartbeat(id: number): Promise<HeartbeatDetailResponse> {
  return fetchJSON<HeartbeatDetailResponse>(`${API_BASE}/heartbeats/${id}`)
}

export function createHeartbeat(data: CreateHeartbeatInput): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateHeartbeat(id: number, data: UpdateHeartbeatInput): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteHeartbeat(id: number): Promise<void> {
  return fetchJSON<void>(`${API_BASE}/heartbeats/${id}`, { method: 'DELETE' })
}

export function pauseHeartbeat(id: number): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}/pause`, { method: 'POST' })
}

export function resumeHeartbeat(id: number): Promise<Heartbeat> {
  return fetchJSON<Heartbeat>(`${API_BASE}/heartbeats/${id}/resume`, { method: 'POST' })
}

export function listExecutions(id: number, params?: { limit?: number; offset?: number }): Promise<ExecutionsResponse> {
  const url = new URL(`${API_BASE}/heartbeats/${id}/executions`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<ExecutionsResponse>(url.toString())
}

export function listPings(id: number, params?: { limit?: number; offset?: number }): Promise<PingsResponse> {
  const url = new URL(`${API_BASE}/heartbeats/${id}/pings`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<PingsResponse>(url.toString())
}
