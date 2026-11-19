import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import type { Heartbeat } from '@/services/heartbeatApi'

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

function row(overrides: Partial<Heartbeat> = {}): Heartbeat {
  return {
    id: 'hb-1',
    ping_token: 'tok-1',
    name: 'nightly backup',
    status: 'up',
    alert_state: 'normal',
    interval_seconds: 3600,
    grace_seconds: 60,
    last_ping_at: '2026-10-24T09:00:00Z',
    next_deadline_at: '2026-10-24T10:01:00Z',
    last_duration_ms: 1200,
    consecutive_failures: 0,
    consecutive_successes: 4,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

function pingReceived(data: Record<string, unknown>) {
  const handler = handlers['heartbeat.ping_received']
  expect(handler, 'the store listens to heartbeat.ping_received').toBeTypeOf('function')
  handler!({ data: JSON.stringify(data) } as MessageEvent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const key of Object.keys(handlers)) delete handlers[key]
  listHeartbeats.mockReset().mockResolvedValue({ heartbeats: [], total: 0 })
})

describe('heartbeats store: heartbeat.ping_received', () => {
  it('moves the last ping, the deadline, the duration and the exit code of the row', () => {
    const store = useHeartbeatsStore()
    store.connectSSE()
    const other = row({ id: 'hb-2', ping_token: 'tok-2' })
    store.heartbeats = [row(), other]

    pingReceived({
      heartbeat_id: 'hb-1',
      ping_type: 'exit_code',
      status: 'up',
      exit_code: 3,
      last_ping_at: '2026-10-24T10:00:00Z',
      next_deadline_at: '2026-10-24T11:01:00Z',
      last_duration_ms: 5400,
      last_exit_code: 3,
    })

    expect(store.heartbeats[0]).toEqual({
      ...row(),
      last_ping_at: '2026-10-24T10:00:00Z',
      next_deadline_at: '2026-10-24T11:01:00Z',
      last_duration_ms: 5400,
      last_exit_code: 3,
    })
    expect(store.heartbeats[1]).toEqual(other)
  })

  it('keeps the values of the row that the event does not carry', () => {
    const store = useHeartbeatsStore()
    store.connectSSE()
    store.heartbeats = [row()]

    pingReceived({ heartbeat_id: 'hb-1', ping_type: 'start', status: 'started', next_deadline_at: '2026-10-24T11:01:00Z' })

    expect(store.heartbeats[0]).toEqual({ ...row(), status: 'started', next_deadline_at: '2026-10-24T11:01:00Z' })
  })
})
