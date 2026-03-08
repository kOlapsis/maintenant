// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export type InsightType =
  | 'port_exposed_all_interfaces'
  | 'database_port_exposed'
  | 'privileged_container'
  | 'host_network_mode'
  | 'service_load_balancer'
  | 'service_node_port'
  | 'missing_network_policy'

export interface SecurityInsight {
  type: InsightType
  severity: string
  container_id: number
  container_name: string
  title: string
  description: string
  details: Record<string, unknown>
  detected_at: string
}

export interface ContainerInsights {
  container_id: number
  container_name: string
  highest_severity: string | null
  count: number
  insights: SecurityInsight[]
}

export interface InsightSummary {
  total_containers_monitored: number
  total_containers_affected: number
  total_insights: number
  by_severity: Record<string, number>
  by_type: Record<string, number>
}

export interface SecurityResponse {
  containers: ContainerInsights[]
  summary: InsightSummary
}

export function fetchInsights(): Promise<SecurityResponse> {
  return apiFetch(`${API_BASE}/security/insights`)
}

export function fetchContainerInsights(containerId: number): Promise<ContainerInsights> {
  return apiFetch(`${API_BASE}/security/insights/${containerId}`)
}

export function fetchSecuritySummary(): Promise<InsightSummary> {
  return apiFetch(`${API_BASE}/security/summary`)
}
