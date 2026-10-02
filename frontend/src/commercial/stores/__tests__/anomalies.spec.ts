// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Capture the SSE handlers the store registers so tests can dispatch events.
const handlers = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (t: string, h: (e: MessageEvent) => void) => handlers.set(t, h),
    off: (t: string) => handlers.delete(t),
    connect: () => {},
    disconnect: () => {},
  },
}))

import { useAnomaliesStore } from '../anomalies'

function fire(type: string, payload: unknown) {
  const h = handlers.get(type)
  if (!h) throw new Error(`no handler for ${type}`)
  h({ data: JSON.stringify(payload) } as MessageEvent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
})

describe('anomalies store live SSE updates', () => {
  it('state_changed inserts then updates a series in place', () => {
    const store = useAnomaliesStore()
    store.connectSSE()

    fire('anomaly.state_changed', {
      scope_type: 'container',
      scope_id: 'compose/app/web',
      metric: 'cpu',
      dimension: '',
      state: 'learning',
      progress: 0.4,
    })
    expect(store.series).toHaveLength(1)
    expect(store.series[0]?.progress).toBe(0.4)
    expect(store.anyReady).toBe(false)

    // The same series turning ready is updated in place, not duplicated.
    fire('anomaly.state_changed', {
      scope_type: 'container',
      scope_id: 'compose/app/web',
      metric: 'cpu',
      dimension: '',
      state: 'ready',
      progress: 1,
    })
    expect(store.series).toHaveLength(1)
    expect(store.series[0]?.state).toBe('ready')
    expect(store.anyReady).toBe(true)
  })

  it('opened adds an active event, closed marks it ended', () => {
    const store = useAnomaliesStore()
    store.connectSSE()

    fire('anomaly.opened', {
      id: 'evt-1',
      scope_type: 'container',
      scope_id: 'compose/app/web',
      metric: 'cpu',
      dimension: '',
      detector: 'spike',
      tier: 'active',
      started_at: 1000,
    })
    expect(store.activeEvents).toHaveLength(1)

    fire('anomaly.closed', { id: 'evt-1', ended_at: 2000 })
    expect(store.activeEvents).toHaveLength(0)
    expect(store.events[0]?.ended_at).toBe(2000)
  })
})
