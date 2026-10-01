// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const handlers = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
const api = vi.hoisted(() => ({
  fetchSwarmInfo: vi.fn(),
  fetchSwarmNodes: vi.fn(),
  fetchSwarmCluster: vi.fn(),
}))

vi.mock('@/services/swarmApi', () => api)
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => handlers.set(type, fn),
    off: (type: string) => handlers.delete(type),
    connected: false,
  },
}))

import { useSwarmStore } from '../swarm'

const worker = {
  id: 'row-1',
  node_id: 'n1',
  hostname: 'node-1',
  role: 'worker',
  status: 'ready',
  availability: 'active',
  engine_version: '27.0.0',
  address: '10.0.0.2',
  task_count: 2,
  first_seen_at: '2026-09-01T00:00:00Z',
  last_seen_at: '2026-09-01T00:00:00Z',
  last_status_change_at: '2026-09-01T00:00:00Z',
}

function emit(type: string, data: unknown) {
  handlers.get(type)?.(new MessageEvent(type, { data: JSON.stringify(data) }))
}

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
  api.fetchSwarmCluster.mockReset().mockResolvedValue(null)
  api.fetchSwarmNodes
    .mockReset()
    .mockResolvedValue({ nodes: [worker], total: 1, manager_count: 0, worker_count: 1 })
})

describe('swarm store: swarm.node_updated', () => {
  it('takes the new role of a promoted node and refreshes the cluster', async () => {
    const store = useSwarmStore()
    await store.loadNodes()
    store.startListening()

    emit('swarm.node_updated', { ...worker, role: 'manager', engine_version: '27.1.0' })

    expect(store.nodes[0]!.role).toBe('manager')
    expect(store.nodes[0]!.engine_version).toBe('27.1.0')
    expect(store.nodes[0]!.first_seen_at).toBe(worker.first_seen_at)
    expect(store.managerCount).toBe(1)
    expect(api.fetchSwarmCluster).toHaveBeenCalled()
    store.stopListening()
  })

  it('reloads the list for a node that joined', async () => {
    const store = useSwarmStore()
    await store.loadNodes()
    store.startListening()
    api.fetchSwarmNodes.mockClear()

    emit('swarm.node_updated', { ...worker, node_id: 'n2', hostname: 'node-2' })

    expect(api.fetchSwarmNodes).toHaveBeenCalledTimes(1)
    store.stopListening()
  })
})
