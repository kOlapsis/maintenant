// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'

// Mock the three Pinia stores the composable reads, so we can drive its inputs
// without a full store/SSE/localStorage setup.
const state = vi.hoisted(() => ({
  runtime: { context: 'docker' as string, metadata: {} as Record<string, unknown> },
  agents: { agents: [] as Array<{ status: string; agent_id: string; detected_runtime: string }> },
  resources: { selected: null as string | null },
}))

vi.mock('@/stores/runtime', () => ({ useRuntimeStore: () => state.runtime }))
vi.mock('@/stores/agents', () => ({ useAgentsStore: () => state.agents }))
vi.mock('@/stores/resources', () => ({ useResourcesStore: () => state.resources }))

import { useFleetRuntimes } from '../useFleetRuntimes'

function runtimes() {
  return useFleetRuntimes().availableRuntimes.value
}

beforeEach(() => {
  state.runtime.context = 'docker'
  state.runtime.metadata = {}
  state.agents.agents = []
  state.resources.selected = null
})

describe('useFleetRuntimes — empty Swarm keeps Docker available', () => {
  it('local Swarm manager with 0 services exposes both swarm and docker', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 0 }
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('local Swarm manager with services does NOT expose docker', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 3 }
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(['swarm'])
  })

  it('missing service_count is treated as 0 (docker stays available)', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = {}
    state.resources.selected = 'local'
    expect(runtimes()).toContain('docker')
  })

  it('"all" scope on an empty Swarm includes both docker and swarm', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 0 }
    state.resources.selected = null
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('does not apply the empty-Swarm rule to a selected remote agent', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 0 }
    state.agents.agents = [{ status: 'active', agent_id: 'agent-1', detected_runtime: 'docker' }]
    state.resources.selected = 'agent-1'
    expect(runtimes()).toEqual(['docker'])
  })

  it('plain docker is unaffected regardless of service_count', () => {
    state.runtime.context = 'docker'
    state.runtime.metadata = { service_count: 0 }
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(['docker'])
  })
})
