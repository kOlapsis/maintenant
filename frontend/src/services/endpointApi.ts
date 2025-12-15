const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface EndpointConfig {
  interval: string
  timeout: string
  failure_threshold: number
  recovery_threshold: number
  method?: string
  expected_status?: string
  tls_verify: boolean
  headers?: Record<string, string>
  max_redirects?: number
}

export interface Endpoint {
  id: number
  container_name: string
  external_id: string
  endpoint_type: 'http' | 'tcp'
  target: string
  label_key: string
  status: 'up' | 'down' | 'unknown'
  alert_state: 'normal' | 'alerting'
  consecutive_failures: number
  consecutive_successes: number
  last_check_at?: string
  last_response_time_ms?: number
  last_http_status?: number
  last_error?: string
  config: EndpointConfig
  active: boolean
  first_seen_at: string
  last_seen_at: string
  orchestration_group?: string
  orchestration_unit?: string
}

export interface CheckResult {
  id: number
  endpoint_id: number
  success: boolean
  response_time_ms: number
  http_status?: number
  error_message?: string
  timestamp: string
}

export interface ListEndpointsParams {
  status?: string
  container?: string
  orchestration_group?: string
  type?: string
  include_inactive?: boolean
}

export interface ListChecksParams {
  limit?: number
  offset?: number
  since?: number
}

export interface EndpointsResponse {
  endpoints: Endpoint[]
  total: number
}

export interface EndpointDetailResponse {
  endpoint: Endpoint
  uptime?: Record<string, number>
}

export interface ChecksResponse {
  endpoint_id: number
  checks: CheckResult[]
  total: number
  has_more: boolean
}

function fetchJSON<T>(url: string): Promise<T> {
  return apiFetch<T>(url)
}

export function listEndpoints(params?: ListEndpointsParams): Promise<EndpointsResponse> {
  const url = new URL(`${API_BASE}/endpoints`, window.location.origin)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.container) url.searchParams.set('container', params.container)
  if (params?.orchestration_group) url.searchParams.set('orchestration_group', params.orchestration_group)
  if (params?.type) url.searchParams.set('type', params.type)
  if (params?.include_inactive) url.searchParams.set('include_inactive', 'true')
  return fetchJSON<EndpointsResponse>(url.toString())
}

export function getEndpoint(id: number): Promise<EndpointDetailResponse> {
  return fetchJSON<EndpointDetailResponse>(`${API_BASE}/endpoints/${id}`)
}

export function listChecks(id: number, params?: ListChecksParams): Promise<ChecksResponse> {
  const url = new URL(`${API_BASE}/endpoints/${id}/checks`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  if (params?.since) url.searchParams.set('since', String(params.since))
  return fetchJSON<ChecksResponse>(url.toString())
}
