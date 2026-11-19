// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import type { Alert } from '@/services/alertApi'
import type { Container } from '@/services/containerApi'

vi.mock('@/services/authGuard', () => ({ probeAuth: vi.fn() }))

class FakeEventSource {
  static instances: FakeEventSource[] = []
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  private handlers = new Map<string, Array<(e: MessageEvent) => void>>()

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener(name: string, handler: (e: MessageEvent) => void) {
    this.handlers.set(name, [...(this.handlers.get(name) ?? []), handler])
  }

  emit(name: string, payload: unknown = {}) {
    for (const h of this.handlers.get(name) ?? []) h(new MessageEvent(name, { data: JSON.stringify(payload) }))
  }

  close() {}

  static get last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]!
  }
}

const api = vi.hoisted(() => ({
  listContainers: vi.fn(),
  listEndpoints: vi.fn(),
}))
vi.mock('@/services/containerApi', () => ({ listContainers: api.listContainers }))
vi.mock('@/services/endpointApi', () => ({ listEndpoints: api.listEndpoints }))

import { useDashboardStore } from '../dashboard'
import { useContainersStore } from '../containers'
import { useAlertsStore } from '../alerts'
import { useEndpointsStore } from '../endpoints'

const emit = (name: string, payload?: unknown) => FakeEventSource.last.emit(name, payload)

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  setActivePinia(createPinia())
  api.listContainers.mockReset().mockResolvedValue({ groups: [], total: 0, archived_count: 0 })
  api.listEndpoints.mockReset().mockResolvedValue({ endpoints: [], total: 0 })
})

afterEach(() => {
  useDashboardStore().disconnectAllSSE()
  vi.unstubAllGlobals()
})

describe('SSE handlers shared between the header and a page', () => {
  it('keeps the header stores live after a page disconnects its own subscription', () => {
    const dashboard = useDashboardStore()
    const containers = useContainersStore()
    const alerts = useAlertsStore()

    dashboard.connectAllSSE()
    containers.connectSSE()
    containers.disconnectSSE()

    containers.groups = [
      { name: 'g', source: 'docker', containers: [{ id: 'c1', state: 'running' } as Container] },
    ]
    emit('container.state_changed', { id: 'c1', state: 'exited', health_status: '', timestamp: 't' })
    expect(containers.groups[0]!.containers[0]!.state).toBe('exited')

    const alert = {
      id: 'a1', source: 'container', alert_type: 'down', severity: 'critical', status: 'active',
      entity_type: 'container', entity_id: 'c1',
    } as Alert
    emit('alert.fired', alert)
    expect(alerts.activeAlerts.critical).toHaveLength(1)
  })
})
