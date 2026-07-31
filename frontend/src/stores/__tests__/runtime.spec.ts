// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const fetchRuntimeStatus = vi.hoisted(() => vi.fn())

vi.mock('@/services/runtimeApi', () => ({ fetchRuntimeStatus }))
vi.mock('@/services/sseBus', () => ({ sseBus: { on: vi.fn(), off: vi.fn(), connected: false } }))
vi.mock('@/composables/useToast', () => ({ showToast: vi.fn() }))

import { useRuntimeStore } from '../runtime'

const OK = {
  context: 'swarm',
  runtime: 'docker',
  connected: true,
  label: 'Services',
  detected_at: '2026-07-30T00:00:00Z',
  metadata: { service_count: 3 },
}

beforeEach(() => {
  setActivePinia(createPinia())
  fetchRuntimeStatus.mockReset()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('runtime store — a failed status fetch does not strip the nav for good', () => {
  it('does not reject: the caller must not see an unhandled rejection', async () => {
    fetchRuntimeStatus.mockRejectedValue(new Error('boom'))
    const store = useRuntimeStore()
    await expect(store.fetchStatus()).resolves.toBeUndefined()
    expect(store.error).toBe('boom')
    store.stopListening()
  })

  it('leaves loaded false while failing, so the nav waits instead of guessing', async () => {
    fetchRuntimeStatus.mockRejectedValue(new Error('boom'))
    const store = useRuntimeStore()
    await store.fetchStatus()
    expect(store.loaded).toBe(false)
    store.stopListening()
  })

  it('retries after a failure and recovers once the API answers', async () => {
    fetchRuntimeStatus.mockRejectedValueOnce(new Error('boom')).mockResolvedValue(OK)
    const store = useRuntimeStore()

    await store.fetchStatus()
    expect(store.loaded).toBe(false)

    await vi.advanceTimersByTimeAsync(1000)

    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(2)
    expect(store.loaded).toBe(true)
    expect(store.context).toBe('swarm')
    expect(store.error).toBeNull()
  })

  it('backs off between attempts and caps the delay', async () => {
    fetchRuntimeStatus.mockRejectedValue(new Error('boom'))
    const store = useRuntimeStore()

    await store.fetchStatus()
    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(1)

    // 1s, then 2s, then 4s — one retry in flight at a time.
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1999)
    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(3)

    // Far past the cap: still one attempt per 30s, not a burst.
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
    expect(fetchRuntimeStatus.mock.calls.length).toBeLessThan(30)

    store.stopListening()
  })

  it('stopListening cancels a pending retry', async () => {
    fetchRuntimeStatus.mockRejectedValue(new Error('boom'))
    const store = useRuntimeStore()

    await store.fetchStatus()
    store.stopListening()

    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchRuntimeStatus).toHaveBeenCalledTimes(1)
  })

  it('a success resets the backoff so a later failure retries fast again', async () => {
    fetchRuntimeStatus.mockRejectedValueOnce(new Error('boom')).mockResolvedValue(OK)
    const store = useRuntimeStore()

    await store.fetchStatus()
    await vi.advanceTimersByTimeAsync(1000)
    expect(store.loaded).toBe(true)

    fetchRuntimeStatus.mockRejectedValueOnce(new Error('again')).mockResolvedValue(OK)
    await store.fetchStatus()
    const callsBefore = fetchRuntimeStatus.mock.calls.length

    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchRuntimeStatus.mock.calls.length).toBe(callsBefore + 1)
  })
})
