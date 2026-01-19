const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface ResourceSnapshot {
  container_id: number
  cpu_percent: number
  mem_used: number
  mem_limit: number
  mem_percent: number
  net_rx_bytes: number
  net_tx_bytes: number
  block_read_bytes: number
  block_write_bytes: number
  timestamp: string
}

export interface ResourceSummary {
  total_cpu_percent: number
  total_mem_used: number
  total_mem_limit: number
  total_mem_percent: number
  total_net_rx_rate: number
  total_net_tx_rate: number
  container_count: number
  disk_total: number
  disk_used: number
  disk_percent: number
  timestamp: string
}

export interface HistoryPoint {
  timestamp: string
  cpu_percent: number
  mem_used: number
  mem_limit: number
  net_rx_bytes: number
  net_tx_bytes: number
  block_read_bytes: number
  block_write_bytes: number
}

export interface HistoryResponse {
  container_id: number
  range: string
  granularity: string
  points: HistoryPoint[]
}

export interface ResourceAlertConfig {
  container_id: number
  cpu_threshold: number
  mem_threshold: number
  enabled: boolean
  alert_state: string
  last_alerted_at: string | null
}

export interface UpdateAlertConfigInput {
  cpu_threshold: number
  mem_threshold: number
  enabled: boolean
}

function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  return apiFetch<T>(url, init)
}

export function getCurrentResources(containerId: number): Promise<ResourceSnapshot> {
  return fetchJSON<ResourceSnapshot>(`${API_BASE}/containers/${containerId}/resources/current`)
}

export function getSummary(): Promise<ResourceSummary> {
  return fetchJSON<ResourceSummary>(`${API_BASE}/resources/summary`)
}

export function getResourceHistory(containerId: number, range: string): Promise<HistoryResponse> {
  return fetchJSON<HistoryResponse>(`${API_BASE}/containers/${containerId}/resources/history?range=${range}`)
}

export function getAlertConfig(containerId: number): Promise<ResourceAlertConfig> {
  return fetchJSON<ResourceAlertConfig>(`${API_BASE}/containers/${containerId}/resources/alerts`)
}

export function updateAlertConfig(containerId: number, input: UpdateAlertConfigInput): Promise<ResourceAlertConfig> {
  return fetchJSON<ResourceAlertConfig>(`${API_BASE}/containers/${containerId}/resources/alerts`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}
