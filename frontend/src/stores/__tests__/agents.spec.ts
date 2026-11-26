// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAgentsStore } from '@/stores/agents'
import type { Agent } from '@/services/agentApi'

vi.mock('@/services/agentApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/agentApi')>()),
  getAgentMetrics: vi.fn().mockResolvedValue(null),
}))

const sse = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => sse.set(type, fn),
    off: (type: string) => sse.delete(type),
    connect: () => {},
    disconnect: () => {},
  },
}))

describe('agents store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    sse.clear()
  })

  it('forgets an unreachable runtime when the agent disconnects', () => {
    const store = useAgentsStore()
    store.connectSSE()
    store.agents = [{ agent_id: 'a1', connection_state: 'connected', runtime_unreachable: true } as Agent]

    sse.get('agent.disconnected')!(new MessageEvent('agent.disconnected', { data: JSON.stringify({ agent_id: 'a1' }) }))

    expect(store.agents[0]).toMatchObject({ connection_state: 'disconnected', runtime_unreachable: false })
  })
})
