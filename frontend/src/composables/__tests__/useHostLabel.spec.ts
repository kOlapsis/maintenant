// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'

const state = vi.hoisted(() => ({
  agents: {
    agents: [] as Array<{ agent_id: string; hostname: string; label: string; status: string }>,
    hasRemoteAgents: false,
  },
  resources: { selected: null as string | null },
}))

vi.mock('@/stores/agents', () => ({ useAgentsStore: () => state.agents }))
vi.mock('@/stores/resources', () => ({ useResourcesStore: () => state.resources }))

import { useHostLabel } from '../useHostLabel'

beforeEach(() => {
  state.agents.agents = [
    { agent_id: 'agent-1', hostname: 'edge-1.lan', label: 'edge', status: 'active' },
    { agent_id: 'agent-2', hostname: 'edge-2.lan', label: '', status: 'active' },
  ]
  state.agents.hasRemoteAgents = true
  state.resources.selected = null
})

describe('hostLabel', () => {
  it('prefers the label the API attached to the row', () => {
    const { hostLabel } = useHostLabel()
    expect(hostLabel('agent-1', 'other.lan', 'from-row')).toBe('from-row')
  })

  it('falls back to the hostname the API attached', () => {
    const { hostLabel } = useHostLabel()
    expect(hostLabel('agent-1', 'other.lan')).toBe('other.lan')
  })

  it('resolves through the agents store when the row carries only an id', () => {
    const { hostLabel } = useHostLabel()
    expect(hostLabel('agent-1')).toBe('edge')
    expect(hostLabel('agent-2')).toBe('edge-2.lan')
  })

  it('falls back to the raw id for an unknown agent', () => {
    const { hostLabel } = useHostLabel()
    expect(hostLabel('agent-ghost')).toBe('agent-ghost')
  })

  it('names the server runtime "Local", sentinel or missing id alike', () => {
    const { hostLabel } = useHostLabel()
    expect(hostLabel('00000000-0000-0000-0000-000000000000')).toBe('Local')
    expect(hostLabel(null)).toBe('Local')
    expect(hostLabel(undefined)).toBe('Local')
  })
})

describe('showHost / hostOf', () => {
  it('names every host, the local one included, on a multi-host install', () => {
    const { showHost, hostOf } = useHostLabel()
    expect(showHost.value).toBe(true)
    expect(hostOf({ agent_id: 'agent-1', agent_hostname: 'edge-1.lan' })).toBe('edge-1.lan')
    expect(hostOf({})).toBe('Local')
  })

  it('stays silent on a single-host install', () => {
    state.agents.agents = []
    state.agents.hasRemoteAgents = false
    const { showHost, hostOf } = useHostLabel()
    expect(showHost.value).toBe(false)
    expect(hostOf({ agent_id: 'agent-1' })).toBeNull()
  })

  it('stays silent once a host is picked as the scope', () => {
    state.resources.selected = 'agent-1'
    const { showHost, hostOf } = useHostLabel()
    expect(showHost.value).toBe(false)
    expect(hostOf({ agent_id: 'agent-1' })).toBeNull()
  })
})
