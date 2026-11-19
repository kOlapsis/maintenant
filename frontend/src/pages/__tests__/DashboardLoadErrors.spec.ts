// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import DashboardPage from '@/pages/DashboardPage.vue'
import VerdictBanner from '@/components/dashboard/VerdictBanner.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { useContainersStore } from '@/stores/containers'
import { useEndpointsStore } from '@/stores/endpoints'
import { useDashboardStore } from '@/stores/dashboard'

const api = vi.hoisted(() => ({
  failing: new Set<string>(),
  containers: [] as unknown[],
}))

function maybeFail(name: string) {
  if (api.failing.has(name)) throw new Error(`${name} unavailable`)
}

vi.mock('@/services/containerApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/containerApi')>()
  return {
    ...actual,
    listContainers: vi.fn(async () => {
      maybeFail('containers')
      return {
        containers: api.containers,
        groups: [{ key: 'g', name: 'g', containers: api.containers }],
        total: api.containers.length,
        archived_count: 0,
      }
    }),
  }
})

vi.mock('@/services/endpointApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/endpointApi')>()
  return {
    ...actual,
    listEndpoints: vi.fn(async () => {
      maybeFail('endpoints')
      return { endpoints: [], total: 0 }
    }),
  }
})

vi.mock('@/services/heartbeatApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/heartbeatApi')>()
  return {
    ...actual,
    listHeartbeats: vi.fn(async () => {
      maybeFail('heartbeats')
      return { heartbeats: [] }
    }),
  }
})

vi.mock('@/services/certificateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/certificateApi')>()
  return {
    ...actual,
    listCertificates: vi.fn(async () => {
      maybeFail('certificates')
      return { certificates: [], total: 0 }
    }),
  }
})

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return {
    ...actual,
    getActiveAlerts: vi.fn(async () => {
      maybeFail('alerts')
      return { critical: [], warning: [], info: [] }
    }),
  }
})

vi.mock('@/services/apiFetch', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/apiFetch')>()
  return { ...actual, apiFetch: vi.fn(async () => ({})) }
})

vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  return {
    sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: ref(true) },
  }
})

vi.mock('@/composables/useEdition', async () => {
  const { ref } = await import('vue')
  return { useEdition: () => ({ edition: ref(null), hasFeature: () => false }) }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, hash: '' }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}))

vi.mock('@/stores/updates', async () => {
  const { defineStore } = await import('pinia')
  return {
    useUpdatesStore: defineStore('updates', () => ({
      criticalCount: 0,
      fetchAllUpdates: vi.fn(),
      fetchSummary: vi.fn(),
    })),
  }
})

vi.mock('@/stores/agents', async () => {
  const { defineStore } = await import('pinia')
  return { useAgentsStore: defineStore('agents', () => ({ agents: [], fetchAgents: vi.fn() })) }
})

const runningContainer = {
  id: 'c1',
  name: 'web',
  image: 'nginx:1',
  state: 'running',
  health_status: 'healthy',
  has_health_check: true,
  archived: false,
  last_state_change_at: '2026-10-13T10:00:00Z',
}

function banner(w: VueWrapper) {
  return w.findComponent(VerdictBanner)
}

describe('DashboardPage load errors', () => {
  let wrapper: VueWrapper

  async function mountPage() {
    wrapper = mount(DashboardPage, {
      global: {
        provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } },
        stubs: {
          AttentionPanel: true,
          KpiStrip: true,
          UpdateSummaryStrip: true,
          RuntimeDegradedBanner: true,
          StatusGrid: { template: '<div><slot name="bar" /></div>' },
        },
      },
    })
    await flushPromises()
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    api.failing.clear()
    api.containers = [runningContainer]
  })

  afterEach(() => {
    wrapper.unmount()
  })

  it('shows data unavailable, not a green verdict, when the container list fails to load', async () => {
    api.failing.add('containers')
    await mountPage()

    const text = banner(wrapper).text()
    expect(text).toContain('Data unavailable')
    expect(text).toContain('containers')
    expect(text).not.toContain('All systems operational')
    expect(wrapper.text()).not.toContain('updated just now')
  })

  it('names every list that failed, and keeps the counters of the lists that loaded', async () => {
    api.failing.add('endpoints')
    await mountPage()

    expect(banner(wrapper).text()).toContain('Data unavailable')
    expect(banner(wrapper).text()).toContain("Couldn't load endpoints")
    expect(banner(wrapper).text()).not.toContain('containers')
    expect(banner(wrapper).text()).toMatch(/1\s*OK/)
    expect(wrapper.text()).not.toContain('updated just now')
  })

  it('shows the retry state and no counters when every list fails', async () => {
    for (const n of ['containers', 'endpoints', 'heartbeats', 'certificates', 'alerts']) api.failing.add(n)
    await mountPage()

    expect(banner(wrapper).text()).toContain('Data unavailable')
    expect(banner(wrapper).text()).not.toContain('OK')
    expect(wrapper.findComponent(ErrorState).exists()).toBe(true)
  })

  it('goes back to the normal verdict once the next load succeeds', async () => {
    api.failing.add('containers')
    await mountPage()
    expect(banner(wrapper).text()).toContain('Data unavailable')

    api.failing.clear()
    await useContainersStore().fetchContainers()
    await flushPromises()

    expect(banner(wrapper).text()).not.toContain('Data unavailable')
    expect(banner(wrapper).text()).toContain('All systems operational')
  })

  it('shows the unavailable state when a later reload fails', async () => {
    await mountPage()
    expect(banner(wrapper).text()).toContain('All systems operational')

    api.failing.add('endpoints')
    await useEndpointsStore().fetchEndpoints()
    await flushPromises()

    expect(banner(wrapper).text()).toContain('Data unavailable')
    expect(banner(wrapper).text()).toContain('endpoints')
  })

  it('keeps the current behaviour when everything loads', async () => {
    await mountPage()

    expect(useDashboardStore().unavailableLists.failed).toEqual([])
    expect(banner(wrapper).text()).toContain('All systems operational')
    expect(banner(wrapper).text()).not.toContain('Data unavailable')
    expect(wrapper.text()).toContain('updated just now')
    expect(wrapper.findComponent(ErrorState).exists()).toBe(false)
  })
})
