// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { computed } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import type { Endpoint } from '@/services/endpointApi'
import type { CertMonitor } from '@/services/certificateApi'
import { certMonitor, endpoint } from '@/components/__tests__/communityLimitFixtures'

// 051-community-label-limits: FR-007 (the state follows the real-time events) and
// the edge case "un endpoint non surveillé n'est compté ni comme en panne ni comme
// opérationnel dans les compteurs du tableau de bord".

const bus = vi.hoisted(() => ({
  handlers: {} as Record<string, Array<(e: MessageEvent) => void>>,
  server: { endpoints: [] as unknown[], certificates: [] as unknown[] },
}))

vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (name: string, fn: (e: MessageEvent) => void) => {
      ;(bus.handlers[name] ??= []).push(fn)
    },
    off: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
    connected: false,
    suspended: false,
  },
}))
vi.mock('@/services/endpointApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/endpointApi')>()),
  listEndpoints: vi.fn(async () => ({ endpoints: bus.server.endpoints, total: bus.server.endpoints.length })),
}))
vi.mock('@/services/certificateApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/certificateApi')>()),
  listCertificates: vi.fn(async () => ({ certificates: bus.server.certificates, total: bus.server.certificates.length })),
}))
vi.mock('@/composables/useHostLabel', () => ({ useHostLabel: () => ({ hostOf: () => null }) }))
vi.mock('@/composables/useFleetRuntimes', () => ({
  useFleetRuntimes: () => ({ availableRuntimes: computed(() => ['docker']) }),
}))

import { useEndpointsStore } from '@/stores/endpoints'
import { useCertificatesStore } from '@/stores/certificates'
import { useDashboardStore } from '@/stores/dashboard'

function emit(name: string, data: unknown) {
  expect(Object.keys(bus.handlers)).toContain(name)
  for (const fn of bus.handlers[name]!) fn({ data: JSON.stringify(data) } as MessageEvent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const k of Object.keys(bus.handlers)) delete bus.handlers[k]
  bus.server.endpoints = []
  bus.server.certificates = []
})

describe('endpoints store: endpoint.limit_changed', () => {
  for (const limited of [true, false]) {
    it(`moves an endpoint to limited=${limited} without a page reload`, async () => {
      const store = useEndpointsStore()
      store.connectSSE()
      store.endpoints = [endpoint({ id: 'a', limited: !limited }), endpoint({ id: 'b', limited: false })]
      bus.server.endpoints = [endpoint({ id: 'a', limited }), endpoint({ id: 'b', limited: false })]

      emit('endpoint.limit_changed', { endpoint_id: 'a', limited })
      await flushPromises()

      expect(store.endpoints.find((e) => e.id === 'a')!.limited).toBe(limited)
      expect(store.endpoints.find((e) => e.id === 'b')!.limited).toBe(false)
    })
  }
})

describe('certificates store: certificate.limit_changed', () => {
  for (const limited of [true, false]) {
    it(`moves a monitor to limited=${limited} without a page reload`, async () => {
      const store = useCertificatesStore()
      store.connectSSE()
      store.certificates = [certMonitor({ id: 'c1', limited: !limited }), certMonitor({ id: 'c2', limited: false })]
      bus.server.certificates = [certMonitor({ id: 'c1', limited }), certMonitor({ id: 'c2', limited: false })]

      emit('certificate.limit_changed', { monitor_id: 'c1', limited })
      await flushPromises()

      expect(store.certificates.find((c) => c.id === 'c1')!.limited).toBe(limited)
      expect(store.certificates.find((c) => c.id === 'c2')!.limited).toBe(false)
    })
  }
})

describe('dashboard counters', () => {
  it('count an element left out neither up nor down, whatever status it last had', () => {
    const endpoints = useEndpointsStore()
    const certificates = useCertificatesStore()
    endpoints.endpoints = [
      endpoint({ id: 'up', status: 'up' }),
      endpoint({ id: 'down', status: 'down' }),
      endpoint({ id: 'left-up', status: 'up', limited: true }),
      endpoint({ id: 'left-down', status: 'down', limited: true }),
      endpoint({ id: 'left-unknown', status: 'unknown', limited: true }),
    ] as Endpoint[]
    certificates.certificates = [
      certMonitor({ id: 'valid', status: 'valid' }),
      certMonitor({ id: 'left-valid', status: 'valid', limited: true }),
      certMonitor({ id: 'left-expired', status: 'expired', limited: true }),
    ] as CertMonitor[]

    const dashboard = useDashboardStore()

    expect(dashboard.endpointSummary.ok).toBe(1)
    expect(dashboard.endpointSummary.down).toBe(1)
    expect(dashboard.endpointSummary.warning).toBe(0)
    expect(dashboard.certificateSummary.ok).toBe(1)
    expect(dashboard.certificateSummary.down).toBe(0)
  })
})
