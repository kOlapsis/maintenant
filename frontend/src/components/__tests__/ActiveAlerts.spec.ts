// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ActiveAlerts from '@/components/ActiveAlerts.vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'

const api = vi.hoisted(() => ({
  getActiveAlerts: vi.fn(),
  listRunsForAlert: vi.fn(),
  getEscalationRun: vi.fn(),
}))

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return { ...actual, getActiveAlerts: api.getActiveAlerts }
})

vi.mock('@/commercial/composables/useEscalationApi', () => ({
  useEscalationApi: () => ({ listRunsForAlert: api.listRunsForAlert, getEscalationRun: api.getEscalationRun }),
}))

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn() },
}))

function mountList() {
  return mount(ActiveAlerts, {
    global: { provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } } },
  })
}

describe('ActiveAlerts', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    for (const fn of Object.values(api)) fn.mockReset()
    api.listRunsForAlert.mockResolvedValue({ runs: [] })
    api.getEscalationRun.mockResolvedValue({ deliveries: [] })
  })

  it('says the active alerts could not be loaded', async () => {
    api.getActiveAlerts.mockRejectedValue(new Error('HTTP 503'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await useAlertsStore().fetchActiveAlerts()

    const wrapper = mountList()

    expect(wrapper.text()).not.toContain('No active alerts')
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
  })

  it('says there is no active alert when the list loaded empty', async () => {
    api.getActiveAlerts.mockResolvedValue({ critical: [], warning: [], info: [] })
    await useAlertsStore().fetchActiveAlerts()

    expect(mountList().text()).toContain('No active alerts')
  })
})
