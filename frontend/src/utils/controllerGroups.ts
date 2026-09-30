// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import type { Container } from '@/services/containerApi'

export interface ControllerGroup {
  kind: string
  name: string
  containers: Container[]
  readyCount: number
  podCount: number | null // null for a Swarm service
}

// controllerGroups gathers containers under their Kubernetes workload or Swarm service.
export function controllerGroups(containers: Container[]): ControllerGroup[] {
  const map = new Map<string, ControllerGroup>()
  for (const c of containers) {
    if (!c.controller_kind) continue
    const isSwarmService = c.controller_kind === 'swarm-service'
    const name = (isSwarmService && c.swarm_service_name) || c.orchestration_unit || c.name
    const key = `${c.controller_kind}/${name}`
    let group = map.get(key)
    if (!group) {
      group = {
        kind: c.controller_kind,
        name,
        containers: [],
        readyCount: isSwarmService ? 0 : (c.ready_count ?? 0),
        podCount: isSwarmService ? null : (c.pod_count ?? 0),
      }
      map.set(key, group)
    }
    group.containers.push(c)
    if (isSwarmService && c.state === 'running') group.readyCount++
  }
  return Array.from(map.values())
}
