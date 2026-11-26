// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import IncidentFeedCard from '@/components/dashboard/IncidentFeedCard.vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'

const api = vi.hoisted(() => ({
  getActiveAlerts: vi.fn(),
  listAlerts: vi.fn(),
}))

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return { ...actual, ...api }
})

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: { value: true } },
}))

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ hasFeature: () => false, whenLoaded: async () => {} }),
}))

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

async function mountCard() {
  const wrapper = mount(IncidentFeedCard, {
    global: {
      provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } },
      stubs: { RouterLink: true },
    },
  })
  await flushPromises()
  return wrapper
}

describe('IncidentFeedCard', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    for (const fn of Object.values(api)) fn.mockReset()
    api.listAlerts.mockResolvedValue({ alerts: [], has_more: false })
    api.getActiveAlerts.mockResolvedValue({ critical: [], warning: [], info: [] })
  })

  it('says the active alerts could not be loaded', async () => {
    api.getActiveAlerts.mockRejectedValue(new Error('HTTP 503'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await useAlertsStore().fetchActiveAlerts()

    const wrapper = await mountCard()

    expect(wrapper.text()).not.toContain('All services operating normally')
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
  })

  it('leaves the active alerts to the app-wide load', async () => {
    await mountCard()

    expect(api.getActiveAlerts).not.toHaveBeenCalled()
  })

  it('says nothing is wrong when the list loaded empty', async () => {
    await useAlertsStore().fetchActiveAlerts()

    expect((await mountCard()).text()).toContain('All services operating normally')
  })

  it('keeps the load error above a live alert and offers to retry', async () => {
    api.getActiveAlerts.mockRejectedValueOnce(new Error('HTTP 502'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAlertsStore()
    await store.fetchActiveAlerts()
    store.activeAlerts = {
      critical: [{ id: 'a1', severity: 'critical', message: 'db is down', entity_name: 'db', fired_at: '2026-11-26T10:00:00Z' } as never],
      warning: [],
      info: [],
    }
    const wrapper = await mountCard()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 502')
    expect(wrapper.text()).toContain('db is down')

    api.getActiveAlerts.mockResolvedValue({ critical: [], warning: [], info: [] })
    await wrapper.findAll('button').find((b) => b.text() === 'Retry')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})
