// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import type { Container } from '@/services/containerApi'

// Must stay in step with ScopeID in internal/commercial/anomaly/series.go, or the link to a container silently drops.
export function containerScopeId(c: Container): string {
  if (c.swarm_service_name || c.controller_kind === 'swarm-service') {
    return `swarm/${c.swarm_service_name ?? ''}`
  }
  if (c.runtime_type === 'kubernetes') {
    return `k8s/${c.namespace ?? ''}/${c.controller_kind ?? ''}/${c.orchestration_unit ?? ''}`
  }
  if (c.orchestration_group && c.orchestration_unit) {
    return `compose/${c.orchestration_group}/${c.orchestration_unit}`
  }
  return `container/${c.name}`
}

export function findContainerForScope(containers: Container[], scopeId: string): Container | null {
  return containers.find((c) => !c.archived && containerScopeId(c) === scopeId) ?? null
}
