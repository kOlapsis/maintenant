// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAlertsStore } from '@/stores/alerts'
import type { Alert } from '@/services/alertApi'

const api = vi.hoisted(() => ({
  getActiveAlerts: vi.fn(),
  listAlerts: vi.fn(),
  listSilenceRules: vi.fn(),
}))

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return { ...actual, ...api }
})

const sse = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => sse.set(type, fn),
    off: (type: string) => sse.delete(type),
    connect: () => {},
    disconnect: () => {},
  },
}))

function alert(id: string, over: Partial<Alert> = {}): Alert {
  return {
    id,
    source: 'container',
    alert_type: 'down',
    severity: 'critical',
    status: 'active',
    message: `${id} is down`,
    entity_type: 'container',
    entity_id: `e-${id}`,
    entity_name: id,
    fired_at: '2026-11-26T10:00:00Z',
    created_at: '2026-11-26T10:00:00Z',
    ...over,
  } as Alert
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}

describe('alerts store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    sse.clear()
    for (const fn of Object.values(api)) fn.mockReset()
    api.listSilenceRules.mockResolvedValue({ rules: [] })
  })

  it('keeps the answer of the latest request for the active alerts', async () => {
    const first = deferred<unknown>()
    const second = deferred<unknown>()
    api.getActiveAlerts.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const store = useAlertsStore()

    const a = store.fetchActiveAlerts()
    const b = store.fetchActiveAlerts()
    second.resolve({ critical: [], warning: [], info: [] })
    await b
    first.resolve({ critical: [alert('stale')], warning: [], info: [] })
    await a

    expect(store.totalActiveCount).toBe(0)
  })

  function emit(type: string, payload: unknown = {}) {
    sse.get(type)!(new MessageEvent(type, { data: JSON.stringify(payload) }))
  }

  it('reloads the history with its filters when the stream reconnects', async () => {
    api.listAlerts.mockResolvedValue({ alerts: [], has_more: false })
    api.getActiveAlerts.mockResolvedValue({ critical: [], warning: [], info: [] })
    const store = useAlertsStore()
    store.connectSSE()
    await store.fetchAlerts({ status: 'resolved', limit: 50 })

    emit('sse.reconnected')

    expect(api.listAlerts).toHaveBeenCalledTimes(2)
    expect(api.listAlerts).toHaveBeenLastCalledWith({ status: 'resolved', limit: 50 })
  })

  it('keeps a new alert out of a history filtered on something else', async () => {
    api.listAlerts.mockResolvedValue({ alerts: [alert('c1')], has_more: false })
    const store = useAlertsStore()
    store.connectSSE()
    await store.fetchAlerts({ severity: 'critical', status: 'active' })

    emit('alert.fired', alert('w1', { severity: 'warning' }))
    emit('alert.silenced', alert('s1', { status: 'silenced' }))
    emit('alert.fired', alert('c2'))

    expect(store.alerts.map((a) => a.id)).toEqual(['c2', 'c1'])
    expect(store.activeAlerts.warning.map((a) => a.id)).toEqual(['w1'])
    expect(store.newAlertCount).toBe(2)
  })

  it('lists a silenced alert once', async () => {
    api.listAlerts.mockResolvedValue({ alerts: [alert('a1')], has_more: false })
    const store = useAlertsStore()
    store.connectSSE()
    await store.fetchAlerts()

    emit('alert.silenced', alert('a1', { status: 'silenced' }))

    expect(store.alerts.map((a) => [a.id, a.status])).toEqual([['a1', 'silenced']])
  })
})
