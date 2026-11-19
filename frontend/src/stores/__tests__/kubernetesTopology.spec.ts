// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const handlers = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
const api = vi.hoisted(() => ({
  fetchWorkloads: vi.fn(),
  fetchPods: vi.fn(),
  fetchClusterOverview: vi.fn(),
  fetchNodes: vi.fn(),
}))

vi.mock('@/services/kubernetesApi', () => api)
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => handlers.set(type, fn),
    off: (type: string) => handlers.delete(type),
    connected: false,
  },
}))

import { useKubernetesStore } from '../kubernetes'
import { useResourcesStore } from '../resources'

const LOCAL_AGENT = '00000000-0000-0000-0000-000000000000'
const REMOTE_AGENT = '11111111-2222-3333-4444-555555555555'

function emitTopologyChanged(agentId: string) {
  handlers.get('kubernetes.topology_changed')?.(
    new MessageEvent('kubernetes.topology_changed', { data: JSON.stringify({ agent_id: agentId }) }),
  )
}

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
  api.fetchWorkloads.mockReset().mockResolvedValue({ groups: [] })
  api.fetchPods.mockReset().mockResolvedValue({ pods: [] })
  api.fetchClusterOverview.mockReset().mockResolvedValue(null)
  api.fetchNodes.mockReset().mockResolvedValue({ nodes: [] })
})

describe('kubernetes store: kubernetes.topology_changed', () => {
  it('refetches in the local scope when the local cluster changed', () => {
    useResourcesStore().selected = 'local'
    const store = useKubernetesStore()
    store.startListening()

    emitTopologyChanged(LOCAL_AGENT)

    expect(api.fetchWorkloads).toHaveBeenCalledTimes(1)
    expect(api.fetchPods).toHaveBeenCalledTimes(1)
    store.stopListening()
  })

  it('ignores another agent in the local scope', () => {
    useResourcesStore().selected = 'local'
    const store = useKubernetesStore()
    store.startListening()

    emitTopologyChanged(REMOTE_AGENT)

    expect(api.fetchWorkloads).not.toHaveBeenCalled()
    store.stopListening()
  })

  it('keeps following a remote agent in its own scope', () => {
    useResourcesStore().selected = REMOTE_AGENT
    const store = useKubernetesStore()
    store.startListening()

    emitTopologyChanged(REMOTE_AGENT)
    emitTopologyChanged(LOCAL_AGENT)

    expect(api.fetchWorkloads).toHaveBeenCalledTimes(1)
    store.stopListening()
  })
})
