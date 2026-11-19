// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/services/authGuard', async (orig) => ({ ...(await orig<object>()), probeAuth: vi.fn() }))

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

  emit(name: string) {
    for (const h of this.handlers.get(name) ?? []) h(new MessageEvent(name, { data: '{}' }))
  }

  close() {}

  static get last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]!
  }
}

const api = vi.hoisted(() => ({
  fetchRuntimeStatus: vi.fn(),
  fetchWorkloads: vi.fn(),
  fetchSwarmInfo: vi.fn(),
  listTriggers: vi.fn(),
  listComponents: vi.fn(),
  fetchInsights: vi.fn(),
  fetchUpdateSummary: vi.fn(),
  getPosture: vi.fn(),
  listSilenceRules: vi.fn(),
  getActiveAlerts: vi.fn(),
}))

vi.mock('@/services/runtimeApi', async (orig) => ({
  ...(await orig<object>()), fetchRuntimeStatus: api.fetchRuntimeStatus,
}))
vi.mock('@/services/kubernetesApi', async (orig) => ({
  ...(await orig<object>()), fetchWorkloads: api.fetchWorkloads,
}))
vi.mock('@/services/swarmApi', async (orig) => ({
  ...(await orig<object>()), fetchSwarmInfo: api.fetchSwarmInfo,
}))
vi.mock('@/services/triggersApi', async (orig) => ({
  ...(await orig<object>()), listTriggers: api.listTriggers,
}))
vi.mock('@/services/statusApi', async (orig) => ({
  ...(await orig<object>()), listComponents: api.listComponents,
}))
vi.mock('@/services/securityApi', async (orig) => ({
  ...(await orig<object>()), fetchInsights: api.fetchInsights,
}))
vi.mock('@/services/updateApi', async (orig) => ({
  ...(await orig<object>()), fetchUpdateSummary: api.fetchUpdateSummary,
}))
vi.mock('@/commercial/services/postureApi', async (orig) => ({
  ...(await orig<object>()), getPosture: api.getPosture,
}))
vi.mock('@/services/alertApi', async (orig) => ({
  ...(await orig<object>()),
  listSilenceRules: api.listSilenceRules,
  getActiveAlerts: api.getActiveAlerts,
}))

import { sseBus } from '@/services/sseBus'
import { useStorageStore } from '../storage'
import { useRuntimeStore } from '../runtime'
import { useKubernetesStore } from '../kubernetes'
import { useSwarmStore } from '../swarm'
import { useTriggersStore } from '../triggers'
import { useStatusAdminStore } from '../statusAdmin'
import { useSecurityStore } from '../security'
import { useUpdatesStore } from '../updates'
import { useAlertsStore } from '../alerts'
import { usePostureStore } from '@/commercial/stores/posture'

const fetchMock = vi.fn()

const cases: Array<[string, () => () => void, () => unknown]> = [
  ['storage', () => useStorageStore().startListening, () => fetchMock],
  ['runtime', () => useRuntimeStore().startListening, () => api.fetchRuntimeStatus],
  ['kubernetes', () => useKubernetesStore().startListening, () => api.fetchWorkloads],
  ['swarm', () => useSwarmStore().startListening, () => api.fetchSwarmInfo],
  ['triggers', () => useTriggersStore().connectSSE, () => api.listTriggers],
  ['statusAdmin', () => useStatusAdminStore().connectSSE, () => api.listComponents],
  ['security', () => useSecurityStore().connectSSE, () => api.fetchInsights],
  ['posture', () => usePostureStore().connectSSE, () => api.getPosture],
  ['updates', () => useUpdatesStore().connectSSE, () => api.fetchUpdateSummary],
  ['alerts (silence rules)', () => useAlertsStore().connectSSE, () => api.listSilenceRules],
]

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockReset().mockResolvedValue({ ok: false })
  for (const fn of Object.values(api)) fn.mockReset().mockRejectedValue(new Error('offline'))
  setActivePinia(createPinia())
  sseBus.connect()
})

afterEach(() => {
  for (let i = 0; i < 3; i++) sseBus.disconnect()
  vi.unstubAllGlobals()
})

describe('stores reload their data when the stream comes back', () => {
  it.each(cases)('%s', (_name, subscribe, loader) => {
    subscribe()()

    expect(loader()).not.toHaveBeenCalled()
    FakeEventSource.last.emit('sse.reconnected')

    expect(loader()).toHaveBeenCalled()
  })

  it('alerts still reload the active alerts', () => {
    useAlertsStore().connectSSE()

    FakeEventSource.last.emit('sse.reconnected')

    expect(api.getActiveAlerts).toHaveBeenCalled()
  })
})
