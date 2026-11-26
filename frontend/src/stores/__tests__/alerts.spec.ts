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
const stream = vi.hoisted(() => ({ connected: { value: false } }))
vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  stream.connected = ref(false)
  return {
    sseBus: {
      on: (type: string, fn: (e: MessageEvent) => void) => sse.set(type, fn),
      off: (type: string) => sse.delete(type),
      connect: () => {},
      disconnect: () => {},
      connected: stream.connected,
    },
  }
})

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
    stream.connected.value = false
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

  it('keeps a good answer that arrives after a later request failed', async () => {
    const first = deferred<unknown>()
    api.getActiveAlerts.mockReturnValueOnce(first.promise).mockRejectedValueOnce(new Error('HTTP 503'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAlertsStore()

    const a = store.fetchActiveAlerts()
    await store.fetchActiveAlerts()
    expect(store.activeAlertsError).toBe('HTTP 503')
    first.resolve({ critical: [alert('a1')], warning: [], info: [] })
    await a

    expect(store.totalActiveCount).toBe(1)
    expect(store.activeAlertsError).toBeNull()
  })

  it('reloads the active alerts when the stream connects after a failed load', async () => {
    api.getActiveAlerts.mockRejectedValueOnce(new Error('HTTP 502'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAlertsStore()
    await store.fetchActiveAlerts()

    api.getActiveAlerts.mockResolvedValue({ critical: [alert('a1')], warning: [], info: [] })
    stream.connected.value = true
    await vi.waitFor(() => expect(store.totalActiveCount).toBe(1))

    expect(store.activeAlertsError).toBeNull()
    stream.connected.value = false
    stream.connected.value = true
    await Promise.resolve()
    expect(api.getActiveAlerts).toHaveBeenCalledTimes(2)
  })

  it('keeps the load error when a live alert arrives', async () => {
    api.getActiveAlerts.mockRejectedValue(new Error('HTTP 502'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAlertsStore()
    store.connectSSE()
    await store.fetchActiveAlerts()

    emit('alert.fired', alert('a1'))

    expect(store.totalActiveCount).toBe(1)
    expect(store.activeAlertsError).toBe('HTTP 502')
  })
})
