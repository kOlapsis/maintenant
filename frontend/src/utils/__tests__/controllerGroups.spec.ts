// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import type { Container } from '@/services/containerApi'
import { controllerGroups } from '../controllerGroups'

function container(overrides: Partial<Container>): Container {
  return {
    id: 'c',
    external_id: 'ext',
    name: 'c',
    image: 'nginx:1.27',
    state: 'running',
    health_status: null,
    has_health_check: false,
    orchestration_group: '',
    orchestration_unit: '',
    custom_group: '',
    is_ignored: false,
    alert_severity: 'warning',
    restart_threshold: 3,
    archived: false,
    first_seen_at: '2026-09-30T12:00:00Z',
    last_state_change_at: '2026-09-30T12:00:00Z',
    ...overrides,
  } as Container
}

describe('controllerGroups', () => {
  it('gathers the tasks of a Swarm service under the service', () => {
    const groups = controllerGroups([
      container({ id: 'a', name: 'prod_web.1.x', controller_kind: 'swarm-service', swarm_service_name: 'prod_web', state: 'running' }),
      container({ id: 'b', name: 'prod_web.2.y', controller_kind: 'swarm-service', swarm_service_name: 'prod_web', state: 'exited' }),
      container({ id: 'c', name: 'prod_api.1.z', controller_kind: 'swarm-service', swarm_service_name: 'prod_api', state: 'running' }),
    ])

    expect(groups).toHaveLength(2)
    const web = groups.find((g) => g.name === 'prod_web')
    expect(web?.kind).toBe('swarm-service')
    expect(web?.containers.map((c) => c.id)).toEqual(['a', 'b'])
    expect(web?.readyCount).toBe(1)
    expect(web?.podCount).toBeNull()
  })

  it('keeps a Kubernetes workload on its own counts', () => {
    const groups = controllerGroups([
      container({ id: 'd', name: 'api', controller_kind: 'Deployment', orchestration_unit: 'api', ready_count: 2, pod_count: 3 }),
      container({ id: 'e', name: 'standalone' }),
    ])

    expect(groups).toEqual([
      expect.objectContaining({ kind: 'Deployment', name: 'api', readyCount: 2, podCount: 3 }),
    ])
  })
})
