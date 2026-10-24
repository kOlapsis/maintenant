// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import type { Heartbeat } from '@/services/heartbeatApi'

// 048-heartbeat-edit: FR-010 and contract sse.md, the lists of every open dashboard follow heartbeat.updated.

const handlers = vi.hoisted(() => ({}) as Record<string, (e: MessageEvent) => void>)
const listHeartbeats = vi.hoisted(() => vi.fn())

vi.mock('@/services/heartbeatApi', () => ({ listHeartbeats }))
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (name: string, fn: (e: MessageEvent) => void) => {
      handlers[name] = fn
    },
    off: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
    connected: false,
    suspended: false,
  },
}))

import { useHeartbeatsStore } from '../heartbeats'

function row(overrides: Partial<Heartbeat>): Heartbeat {
  return {
    id: 'hb-1',
    ping_token: 'tok-1',
    name: 'nightly backup',
    status: 'up',
    alert_state: 'normal',
    interval_seconds: 86400,
    grace_seconds: 600,
    last_ping_at: '2026-10-24T09:00:00Z',
    next_deadline_at: '2026-10-25T09:10:00Z',
    consecutive_failures: 0,
    consecutive_successes: 4,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

function updated(data: Record<string, unknown>) {
  const handler = handlers['heartbeat.updated']
  expect(handler, 'the store listens to heartbeat.updated').toBeTypeOf('function')
  handler!({ data: JSON.stringify(data) } as MessageEvent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const key of Object.keys(handlers)) delete handlers[key]
  listHeartbeats.mockReset().mockResolvedValue({ heartbeats: [], total: 0 })
})

describe('heartbeats store: heartbeat.updated', () => {
  it.each([
    { name: 'db dump', interval_seconds: 3600, grace_seconds: 300, next_deadline_at: '2026-10-24T10:05:00Z' },
    { name: 'nightly backup (eu)', interval_seconds: 43200, grace_seconds: 0, next_deadline_at: '2026-10-24T21:00:00Z' },
  ])('replaces the edited values of the row in place: $name', (change) => {
    const store = useHeartbeatsStore()
    store.connectSSE()
    const other = row({ id: 'hb-2', ping_token: 'tok-2', name: 'hourly sync', interval_seconds: 3600, grace_seconds: 60 })
    store.heartbeats = [row({}), other]

    updated({ heartbeat_id: 'hb-1', ...change })

    expect(store.heartbeats).toHaveLength(2)
    expect(store.heartbeats[0]).toEqual({ ...row({}), ...change })
    expect(store.heartbeats[1]).toEqual(other)
    expect(listHeartbeats, 'a known row is patched, not reloaded').not.toHaveBeenCalled()
  })

  it('reloads the list when the edited heartbeat is not loaded', async () => {
    const store = useHeartbeatsStore()
    store.connectSSE()
    store.heartbeats = [row({})]

    updated({ heartbeat_id: 'hb-unknown', name: 'db dump', interval_seconds: 3600, grace_seconds: 300 })
    await Promise.resolve()

    expect(listHeartbeats).toHaveBeenCalledTimes(1)
  })
})
