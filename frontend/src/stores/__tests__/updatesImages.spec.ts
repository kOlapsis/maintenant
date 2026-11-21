// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUpdatesStore } from '@/stores/updates'

const api = vi.hoisted(() => ({
  fetchImages: vi.fn(),
  fetchHostOS: vi.fn(),
  fetchUpdates: vi.fn(),
  fetchUpdateSummary: vi.fn(),
}))

const sse = vi.hoisted(() => ({ handlers: {} as Record<string, () => void> }))

vi.mock('@/services/updateApi', () => api)
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: vi.fn((name: string, fn: () => void) => { sse.handlers[name] = fn }),
    off: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
  },
}))
vi.mock('@/composables/useEdition', () => ({ useEdition: () => ({ hasFeature: () => true }) }))

const image = {
  container_id: 'c1',
  container_name: 'db',
  image: 'postgres:18',
  support: { state: 'supported', product: 'postgresql', cycle: '18' },
}

describe('updates store images', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    for (const fn of Object.values(api)) fn.mockReset()
    api.fetchHostOS.mockResolvedValue({ hosts: [], eol_table: null })
    api.fetchUpdates.mockResolvedValue({ updates: [], last_scan: '', next_scan: '' })
    api.fetchUpdateSummary.mockResolvedValue({})
  })

  it('loads the images', async () => {
    api.fetchImages.mockResolvedValue({ images: [image] })
    const store = useUpdatesStore()
    await store.fetchImages()
    expect(store.images).toEqual([image])
  })

  it('treats a missing list as empty', async () => {
    api.fetchImages.mockResolvedValue({})
    const store = useUpdatesStore()
    await store.fetchImages()
    expect(store.images).toEqual([])
  })

  it('keeps the previous list when the request fails', async () => {
    api.fetchImages.mockResolvedValueOnce({ images: [image] })
    const store = useUpdatesStore()
    await store.fetchImages()
    api.fetchImages.mockRejectedValueOnce(new Error('HTTP 500'))
    await expect(store.fetchImages()).resolves.toBeUndefined()
    expect(store.images).toEqual([image])
  })

  it.each(['update.scan_completed', 'agent.updated', 'sse.reconnected'])(
    'refreshes the images with the hosts on %s',
    async (event) => {
      api.fetchImages.mockResolvedValue({ images: [image] })
      const store = useUpdatesStore()
      store.connectSSE()
      sse.handlers[event]!()
      await vi.waitFor(() => expect(store.images).toEqual([image]))
      expect(api.fetchHostOS).toHaveBeenCalledTimes(1)
      expect(api.fetchImages).toHaveBeenCalledTimes(1)
    },
  )
})
