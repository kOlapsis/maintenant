// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useStorageStore } from '../storage'
import { ApiError } from '@/services/apiFetch'

const handlers = new Map<string, (e: MessageEvent) => void>()

vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (name: string, h: (e: MessageEvent) => void) => handlers.set(name, h),
    off: (name: string) => handlers.delete(name),
  },
}))

function emit(name: string, payload: unknown) {
  const h = handlers.get(name)
  if (!h) throw new Error(`no handler for ${name}`)
  h({ data: JSON.stringify(payload) } as MessageEvent)
}

describe('storage store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    handlers.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('starts connected, so an install on the local file never sees a banner', () => {
    const store = useStorageStore()
    expect(store.connected).toBe(true)
    expect(store.engine).toBe('sqlite')
    expect(store.peers).toBe(0)
  })

  it('reads the initial state from the health diagnostic', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ storage: { engine: 'postgres', connected: true, peers: 2 } }),
      }),
    )

    const store = useStorageStore()
    await store.fetchStatus()

    expect(store.engine).toBe('postgres')
    expect(store.connected).toBe(true)
    expect(store.peers).toBe(2)
  })

  it('follows the outage and the recovery announced over SSE', () => {
    const store = useStorageStore()
    store.startListening()

    emit('storage.availability_changed', { engine: 'postgres', connected: false })
    expect(store.connected).toBe(false)
    expect(store.engine).toBe('postgres')

    emit('storage.availability_changed', { engine: 'postgres', connected: true })
    expect(store.connected).toBe(true)
  })

  it('keeps its state when the health endpoint is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')))

    const store = useStorageStore()
    await store.fetchStatus()

    expect(store.connected).toBe(true)
  })
})

describe('ApiError.isStorageOutage', () => {
  it('recognises the outage the interface must ride out', () => {
    const outage = new ApiError(503, { code: 'STORAGE_UNAVAILABLE', message: 'x' }, 'x')
    expect(outage.isStorageOutage).toBe(true)
  })

  it('does not mistake other failures for one', () => {
    expect(new ApiError(500, { code: 'INTERNAL_ERROR', message: 'x' }, 'x').isStorageOutage).toBe(false)
    expect(new ApiError(503, { code: 'SOMETHING_ELSE', message: 'x' }, 'x').isStorageOutage).toBe(false)
  })
})
