// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const handlers = vi.hoisted(() => new Map<string, Array<() => void>>())
const listComponents = vi.hoisted(() => vi.fn())
const listIncidents = vi.hoisted(() => vi.fn())
const edition = vi.hoisted(() => ({ loaded: () => {}, features: false }))

vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (name: string, fn: () => void) => handlers.set(name, [...(handlers.get(name) ?? []), fn]),
    off: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
  },
}))
vi.mock('@/services/statusApi', () => ({
  listComponents,
  listIncidents,
  listMaintenance: vi.fn(),
  listSubscribers: vi.fn(),
}))
vi.mock('@/composables/useEdition', () => {
  const whenLoaded = new Promise<void>((resolve) => (edition.loaded = resolve))
  return { useEdition: () => ({ hasFeature: () => edition.features, whenLoaded: () => whenLoaded }) }
})

import { useStatusAdminStore } from '../statusAdmin'

beforeEach(() => {
  setActivePinia(createPinia())
  handlers.clear()
  listComponents.mockReset().mockResolvedValue([])
})

describe('status admin store', () => {
  it.each(['status.component_created', 'status.component_updated', 'status.component_deleted'])(
    'refetches the components on %s',
    (name) => {
      useStatusAdminStore().connectSSE()
      for (const fn of handlers.get(name) ?? []) fn()
      expect(listComponents).toHaveBeenCalledTimes(1)
    },
  )

  it('loads the incidents once a late edition is known', async () => {
    listIncidents.mockResolvedValue({ incidents: [{ id: 'i1' }], total: 1 })
    const store = useStatusAdminStore()

    const pending = store.fetchIncidents()
    await Promise.resolve()
    expect(listIncidents).not.toHaveBeenCalled()

    edition.features = true
    edition.loaded()
    await pending

    expect(store.incidents).toHaveLength(1)
  })
})
