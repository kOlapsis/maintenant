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
import { apiFetch } from './apiFetch'

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
  agent_id: string
  available: boolean
  total_cpu_percent: number
  cpu_count: number
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

// ResourceHost is one selectable host in the resources view: the local server
// (agent_id === '') or a remote agent. Metrics are zero when available is false.
export interface ResourceHost {
  agent_id: string
  hostname: string
  label: string
  is_local: boolean
  available: boolean
  cpu_percent: number
  mem_used: number
  mem_total: number
  mem_percent: number
  disk_total: number
  disk_used: number
  disk_percent: number
  container_count: number
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

// getSummary scopes to a host: agentId omitted/empty => local server,
// 'local' => local server, '<id>' => that agent.
export function getSummary(agentId?: string): Promise<ResourceSummary> {
  const qs = agentId ? `?agent_id=${encodeURIComponent(agentId)}` : ''
  return fetchJSON<ResourceSummary>(`${API_BASE}/resources/summary${qs}`)
}

export function getResourceHosts(): Promise<{ hosts: ResourceHost[] }> {
  return fetchJSON<{ hosts: ResourceHost[] }>(`${API_BASE}/resources/hosts`)
}

export function getResourceHistory(containerId: number, range: string): Promise<HistoryResponse> {
  return fetchJSON<HistoryResponse>(`${API_BASE}/containers/${containerId}/resources/history?range=${range}`)
}

export function getAlertConfig(containerId: number): Promise<ResourceAlertConfig> {
  return fetchJSON<ResourceAlertConfig>(`${API_BASE}/containers/${containerId}/resources/alerts`)
}

export interface TopConsumerApi {
  container_id: number
  container_name: string
  value: number
  percent: number
  rank: number
}

export interface TopConsumerResponse {
  metric: string
  period?: string
  consumers: TopConsumerApi[]
}

// getTopConsumers ranks containers. agentId scopes by host ('local' for the
// local server, '<id>' for an agent); omitted => all hosts combined.
export function getTopConsumers(metric: string, period: string, limit = 5, agentId?: string): Promise<TopConsumerResponse> {
  const host = agentId ? `&agent_id=${encodeURIComponent(agentId)}` : ''
  return fetchJSON<TopConsumerResponse>(`${API_BASE}/resources/top?metric=${metric}&period=${period}&limit=${limit}${host}`)
}

export function updateAlertConfig(containerId: number, input: UpdateAlertConfigInput): Promise<ResourceAlertConfig> {
  return fetchJSON<ResourceAlertConfig>(`${API_BASE}/containers/${containerId}/resources/alerts`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}
