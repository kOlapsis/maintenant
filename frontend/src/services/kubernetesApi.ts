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

import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface K8sCondition {
  type: string
  status: string
  reason: string
  message: string
  last_transition: string
}

export interface K8sContainerStatus {
  name: string
  image: string
  ready: boolean
  restart_count: number
  state: string
  state_reason: string
  started_at: string | null
}

export interface K8sWorkload {
  id: string
  name: string
  namespace: string
  kind: 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Job'
  images: string[]
  ready_replicas: number
  desired_replicas: number
  status: 'healthy' | 'degraded' | 'progressing' | 'failed'
  conditions: K8sCondition[]
  labels: Record<string, string>
  created_at: string
  last_transition: string
}

export interface K8sWorkloadGroup {
  namespace: string
  workloads: K8sWorkload[]
}

export interface K8sPod {
  name: string
  namespace: string
  status: string
  status_reason: string
  restart_count: number
  node_name: string
  pod_ip: string
  host_ip: string
  containers: K8sContainerStatus[]
  workload_ref: string
  created_at: string
}

export interface K8sEvent {
  type: string
  reason: string
  message: string
  source: string
  first_seen: string
  last_seen: string
  count: number
}

export interface K8sWorkloadListResponse {
  groups: K8sWorkloadGroup[]
  total: number
}

export interface K8sWorkloadDetailResponse {
  workload: K8sWorkload
  pods: K8sPod[]
  events: K8sEvent[]
}

export interface K8sPodListResponse {
  pods: K8sPod[]
  total: number
}

export interface K8sPodDetailResponse {
  pod: K8sPod
  events: K8sEvent[]
}

export function fetchNamespaces(): Promise<{ namespaces: string[]; total: number }> {
  return apiFetch<{ namespaces: string[]; total: number }>(`${API_BASE}/kubernetes/namespaces`)
}

export function fetchWorkloads(params?: {
  namespaces?: string
  kind?: string
  status?: string
}): Promise<K8sWorkloadListResponse> {
  const url = new URL(`${API_BASE}/kubernetes/workloads`, window.location.origin)
  if (params?.namespaces) url.searchParams.set('namespaces', params.namespaces)
  if (params?.kind) url.searchParams.set('kind', params.kind)
  if (params?.status) url.searchParams.set('status', params.status)
  return apiFetch<K8sWorkloadListResponse>(url.toString())
}

export function fetchWorkloadDetail(id: string): Promise<K8sWorkloadDetailResponse> {
  return apiFetch<K8sWorkloadDetailResponse>(
    `${API_BASE}/kubernetes/workloads/${encodeURIComponent(id)}`,
  )
}

export function fetchPods(params?: {
  namespaces?: string
  workload?: string
  node?: string
  status?: string
}): Promise<K8sPodListResponse> {
  const url = new URL(`${API_BASE}/kubernetes/pods`, window.location.origin)
  if (params?.namespaces) url.searchParams.set('namespaces', params.namespaces)
  if (params?.workload) url.searchParams.set('workload', params.workload)
  if (params?.node) url.searchParams.set('node', params.node)
  if (params?.status) url.searchParams.set('status', params.status)
  return apiFetch<K8sPodListResponse>(url.toString())
}

export function fetchPodDetail(namespace: string, name: string): Promise<K8sPodDetailResponse> {
  return apiFetch<K8sPodDetailResponse>(
    `${API_BASE}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`,
  )
}

// --- Per-workload resource metrics (Enterprise) ---

export interface K8sPodResourceEntry {
  name: string
  namespace: string
  node_name: string
  status: string
  cpu_millicores: number | null
  mem_bytes: number | null
  mem_limit_bytes: number | null
  mem_percent: number | null
  timestamp: string | null
}

export interface K8sWorkloadResourcesResponse {
  metrics_available: boolean
  message?: string
  workload_id?: string
  pods: K8sPodResourceEntry[]
}

export function fetchWorkloadResources(id: string): Promise<K8sWorkloadResourcesResponse> {
  return apiFetch<K8sWorkloadResourcesResponse>(
    `${API_BASE}/kubernetes/workloads/${encodeURIComponent(id)}/resources`,
  )
}

// --- Cluster overview (Enterprise) ---

export interface K8sPodStatusBreakdown {
  running: number
  pending: number
  failed: number
  succeeded: number
  unknown: number
}

export interface K8sNamespaceSummary {
  name: string
  workload_count: number
  pod_count: number
  healthy: boolean
}

export interface K8sClusterOverview {
  namespace_count: number
  node_count: number
  node_ready_count: number
  pod_status: K8sPodStatusBreakdown
  workload_count: number
  workload_healthy: number
  cluster_health: 'healthy' | 'degraded' | 'unhealthy'
  namespaces: K8sNamespaceSummary[]
}

export function fetchClusterOverview(): Promise<K8sClusterOverview> {
  return apiFetch<K8sClusterOverview>(`${API_BASE}/kubernetes/cluster`)
}

// --- Nodes (Enterprise) ---

export interface K8sNodeResponse {
  name: string
  roles: string[]
  conditions: K8sCondition[]
  status: string
  capacity: { cpu_millicores: number; memory_bytes: number; pods: number }
  allocatable: { cpu_millicores: number; memory_bytes: number; pods: number }
  running_pods: number
  kubernetes_version: string
  os_image: string
  architecture: string
  created_at: string
}

export interface K8sNodeListResponse {
  nodes: K8sNodeResponse[]
  total: number
}

export interface K8sNodeDetailResponse {
  node: K8sNodeResponse
  pods: K8sPod[]
  events: K8sEvent[]
}

export function fetchNodes(): Promise<K8sNodeListResponse> {
  return apiFetch<K8sNodeListResponse>(`${API_BASE}/kubernetes/nodes`)
}

export function fetchNodeDetail(name: string): Promise<K8sNodeDetailResponse> {
  return apiFetch<K8sNodeDetailResponse>(
    `${API_BASE}/kubernetes/nodes/${encodeURIComponent(name)}`,
  )
}
