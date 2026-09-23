// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch } from './apiFetch'

export type RuntimeContextValue = 'docker' | 'swarm' | 'kubernetes'
export type RuntimeValue = 'docker' | 'kubernetes'

export interface SwarmMetadata {
  cluster_id: string
  is_manager: boolean
  manager_count: number
  worker_count: number
  service_count: number
}

export interface KubernetesMetadata {
  namespace_count: number
  node_count: number
}

export type DockerMetadata = Record<string, never>

export interface RuntimeStatus {
  runtime: RuntimeValue
  context: RuntimeContextValue
  connected: boolean
  label: string
  detected_at: string
  metadata: SwarmMetadata | KubernetesMetadata | DockerMetadata
}

export function fetchRuntimeStatus(): Promise<RuntimeStatus> {
  return apiFetch<RuntimeStatus>(`${API_BASE}/runtime/status`)
}
