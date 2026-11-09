// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { certMonitor, endpoint } from '@/components/__tests__/communityLimitFixtures'

const bus = vi.hoisted(() => ({ handlers: {} as Record<string, Array<(e: MessageEvent) => void>> }))

vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (name: string, fn: (e: MessageEvent) => void) => {
      ;(bus.handlers[name] ??= []).push(fn)
    },
    off: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
    connected: false,
  },
}))

import { useEndpointsStore } from '@/stores/endpoints'
import { useCertificatesStore } from '@/stores/certificates'
import { useDashboardStore } from '@/stores/dashboard'

function emit(name: string, data: unknown) {
  for (const fn of bus.handlers[name]!) fn({ data: JSON.stringify(data) } as MessageEvent)
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const k of Object.keys(bus.handlers)) delete bus.handlers[k]
})

describe('limit_changed events', () => {
  it('reset an endpoint to unknown when it becomes limited, and ignore an unknown id', () => {
    const store = useEndpointsStore()
    store.connectSSE()
    store.endpoints = [endpoint({ id: 'a', status: 'down' })]

    emit('endpoint.limit_changed', { endpoint_id: 'missing', limited: true })
    expect(store.endpoints).toHaveLength(1)
    expect(store.endpoints[0]!.limited).toBe(false)

    emit('endpoint.limit_changed', { endpoint_id: 'a', limited: true })
    expect(store.endpoints[0]).toMatchObject({ limited: true, status: 'unknown' })
  })

  it('reset a certificate monitor to unknown when it becomes limited, and ignore an unknown id', () => {
    const store = useCertificatesStore()
    store.connectSSE()
    store.certificates = [certMonitor({ id: 'c', status: 'valid' })]

    emit('certificate.limit_changed', { monitor_id: 'missing', limited: true })
    expect(store.certificates[0]!.limited).toBe(false)

    emit('certificate.limit_changed', { monitor_id: 'c', limited: true })
    expect(store.certificates[0]).toMatchObject({ limited: true, status: 'unknown' })
  })
})

describe('dashboard unified monitors', () => {
  it('list an element left out as unknown whatever its last check said', () => {
    useEndpointsStore().endpoints = [endpoint({ id: 'e', status: 'up', limited: true })]
    useCertificatesStore().certificates = [
      certMonitor({ id: 'c', status: 'valid', limited: true, latest_check: { days_remaining: 90 } as never }),
    ]
    const monitors = useDashboardStore().monitors
    expect(monitors.find((m) => m.id === 'endpoint:e')!.status).toBe('unknown')
    expect(monitors.find((m) => m.id === 'certificate:c')!.status).toBe('unknown')
  })
})
