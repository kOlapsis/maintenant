// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'

// Mock the three Pinia stores the composable reads, so we can drive its inputs
// without a full store/SSE/localStorage setup.
const state = vi.hoisted(() => ({
  runtime: { context: 'docker' as string, metadata: {} as Record<string, unknown>, loaded: true },
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
  state.runtime.loaded = true
  state.agents.agents = []
  state.resources.selected = null
})

describe('useFleetRuntimes — Swarm keeps Docker available', () => {
  it('local Swarm manager with 0 services exposes both swarm and docker', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 0 }
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  // Issue #28: the Containers entry vanished from the sidebar as soon as one
  // service was deployed, while /containers still worked by direct URL.
  it('local Swarm manager with services still exposes docker', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 3 }
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('the service count no longer decides anything', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = {}
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('"all" scope on a busy Swarm includes both docker and swarm', () => {
    state.runtime.context = 'swarm'
    state.runtime.metadata = { service_count: 7 }
    state.resources.selected = null
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('a selected Swarm agent exposes docker too', () => {
    state.runtime.context = 'docker'
    state.agents.agents = [{ status: 'active', agent_id: 'agent-1', detected_runtime: 'swarm' }]
    state.resources.selected = 'agent-1'
    expect(runtimes()).toEqual(expect.arrayContaining(['swarm', 'docker']))
  })

  it('a selected Docker agent is unaffected by the local Swarm context', () => {
    state.runtime.context = 'swarm'
    state.agents.agents = [{ status: 'active', agent_id: 'agent-1', detected_runtime: 'docker' }]
    state.resources.selected = 'agent-1'
    expect(runtimes()).toEqual(['docker'])
  })

  it('a Kubernetes scope does not gain docker', () => {
    state.runtime.context = 'kubernetes'
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(['kubernetes'])
  })

  it('plain docker stays exactly docker', () => {
    state.runtime.context = 'docker'
    state.resources.selected = 'local'
    expect(runtimes()).toEqual(['docker'])
  })
})

describe('useFleetRuntimes — no flash before the runtime is known', () => {
  it('local scope contributes nothing until the status is loaded', () => {
    state.runtime.loaded = false
    state.runtime.context = 'docker'
    state.resources.selected = 'local'
    expect(runtimes()).toEqual([])
  })

  it('"all" scope ignores the unloaded local runtime but keeps known agents', () => {
    state.runtime.loaded = false
    state.runtime.context = 'docker'
    state.agents.agents = [{ status: 'active', agent_id: 'agent-1', detected_runtime: 'docker' }]
    state.resources.selected = null
    expect(runtimes()).toEqual(['docker'])
  })
})
