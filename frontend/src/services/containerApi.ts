import type { ContainerGroup } from '@/stores/containers.ts'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface Container {
  id: number
  external_id: string
  name: string
  image: string
  state: string
  health_status: string | null
  has_health_check: boolean
  orchestration_group?: string
  orchestration_unit?: string
  custom_group?: string
  is_ignored: boolean
  alert_severity: string
  restart_threshold: number
  alert_channels?: string
  archived: boolean
  first_seen_at: string
  last_state_change_at: string
  archived_at?: string
  uptime_24h?: number
  runtime_type?: string
  error_detail?: string
  controller_kind?: string
  namespace?: string
  pod_count?: number
  ready_count?: number
}

export interface ContainerListResponse {
  containers: Container[]
  groups: ContainerGroup[]
  total: number
  archived_count: number
}

export interface ContainerDetailResponse extends Container {
  uptime?: {
    '24h': number | null
    '7d': number | null
    '30d': number | null
    '90d': number | null
  }
  recent_transitions?: StateTransition[]
  container_names?: string[]
}

export interface StateTransition {
  id: number
  previous_state: string
  new_state: string
  previous_health?: string
  new_health?: string
  exit_code?: number
  log_snippet?: string
  timestamp: string
}

export interface TransitionsResponse {
  container_id: number
  transitions: StateTransition[]
  total: number
  has_more: boolean
}

export interface ListContainersParams {
  archived?: boolean
  group?: string
  state?: string
}

export interface ListTransitionsParams {
  since?: string
  until?: string
  limit?: number
  offset?: number
}

import { apiFetch } from './apiFetch'

function fetchJSON<T>(url: string): Promise<T> {
  return apiFetch<T>(url)
}

export function listContainers(params?: ListContainersParams): Promise<ContainerListResponse> {
  const url = new URL(`${API_BASE}/containers`, window.location.origin)
  if (params?.archived) url.searchParams.set('archived', 'true')
  if (params?.group) url.searchParams.set('group', params.group)
  if (params?.state) url.searchParams.set('state', params.state)
  return fetchJSON<ContainerListResponse>(url.toString())
}

export function getContainer(id: number): Promise<ContainerDetailResponse> {
  return fetchJSON<ContainerDetailResponse>(`${API_BASE}/containers/${id}`)
}

export function listTransitions(
  id: number,
  params?: ListTransitionsParams,
): Promise<TransitionsResponse> {
  const url = new URL(`${API_BASE}/containers/${id}/transitions`, window.location.origin)
  if (params?.since) url.searchParams.set('since', params.since)
  if (params?.until) url.searchParams.set('until', params.until)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<TransitionsResponse>(url.toString())
}

