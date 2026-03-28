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

export type RuntimeContextValue = 'docker' | 'swarm' | 'kubernetes'
export type RuntimeValue = 'docker' | 'kubernetes'

export interface SwarmMetadata {
  cluster_id: string
  is_manager: boolean
  manager_count: number
  worker_count: number
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
