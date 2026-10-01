// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const handlers = vi.hoisted(() => new Map<string, Array<() => void>>())
const listComponents = vi.hoisted(() => vi.fn())

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
  listIncidents: vi.fn(),
  listMaintenance: vi.fn(),
  listSubscribers: vi.fn(),
}))
vi.mock('@/composables/useEdition', () => ({ useEdition: () => ({ hasFeature: () => false }) }))

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
})
