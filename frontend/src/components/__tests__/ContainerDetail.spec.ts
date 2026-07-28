// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ContainerDetail from '@/components/ContainerDetail.vue'
import type { ContainerDetailResponse } from '@/services/containerApi'

vi.mock('@/services/containerApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/containerApi')>()
  return {
    ...actual,
    getContainer: vi.fn(),
    listTransitions: vi.fn(),
    deleteContainer: vi.fn(),
  }
})

vi.mock('@/services/uptimeApi', () => ({
  fetchContainerDailyUptime: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/services/swarmApi', () => ({
  fetchSwarmServiceDetail: vi.fn().mockResolvedValue(null),
}))

vi.mock('@/composables/useLogStream', () => ({
  useLogStream: () => ({
    lines: { value: [] },
    connect: vi.fn(),
    disconnect: vi.fn(),
  }),
}))

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ hasFeature: () => true, edition: 'pro' }),
}))

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn() },
}))

// Mocked at module level, not stubbed: importing the real charts pulls in uPlot,
// which touches matchMedia at import time and is unavailable under jsdom.
vi.mock('@/components/ResourceCharts.vue', () => ({
  default: { template: '<div data-test="resource-charts" />' },
}))
vi.mock('@/components/ResourceAlertConfig.vue', () => ({
  default: { template: '<div data-test="resource-alerts" />' },
}))

import { getContainer, listTransitions } from '@/services/containerApi'

const container = {
  id: 'ctr1',
  external_id: '0a65480373d9abcdef',
  name: 'authlab-maintenant',
  image: 'ghcr.io/kolapsis/maintenant',
  state: 'running',
  health_status: 'healthy',
  has_health_check: true,
  is_ignored: false,
  alert_severity: 'warning',
  restart_threshold: 3,
  archived: false,
  first_seen_at: '2026-07-27T22:05:59Z',
  last_state_change_at: '2026-07-27T22:05:59Z',
} as ContainerDetailResponse

function mountDetail() {
  vi.mocked(getContainer).mockResolvedValue(container)
  vi.mocked(listTransitions).mockResolvedValue({
    container_id: 'ctr1',
    transitions: [],
    total: 0,
    has_more: false,
  })
  return mount(ContainerDetail, {
    props: { containerId: 'ctr1' },
    global: {
      plugins: [createPinia()],
      stubs: {
        // Pass-through so the gated children render, while exposing the key.
        FeatureGate: {
          props: ['feature'],
          template: '<div :data-feature="feature"><slot /></div>',
        },
        LogViewer: true,
        LogExpandedView: true,
        ContainerEventTimeline: true,
        UptimeBar90: true,
        SecurityInsightList: true,
        PostureScoreBadge: true,
        PostureCategoryBreakdown: true,
      },
    },
  })
}

async function settle(wrapper: ReturnType<typeof mountDetail>) {
  await new Promise((r) => setTimeout(r, 50))
  await wrapper.vm.$nextTick()
}

function resourcesTab(wrapper: ReturnType<typeof mountDetail>) {
  return wrapper.findAll('button').find((b) => b.text().includes('Resources'))
}

describe('ContainerDetail — Resources tab', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('offers a Resources tab alongside Details and Logs', async () => {
    const wrapper = mountDetail()
    await settle(wrapper)

    const labels = wrapper.findAll('button').map((b) => b.text())
    expect(labels).toContain('Details')
    expect(labels).toContain('Logs')
    expect(labels).toContain('Resources')
  })

  // The charts fetch their history on mount, so keeping them out of the DOM
  // until the tab is opened is what stops every panel from hitting the API.
  it('does not mount the charts until the tab is opened', async () => {
    const wrapper = mountDetail()
    await settle(wrapper)

    expect(wrapper.find('[data-test="resource-charts"]').exists()).toBe(false)
  })

  it('renders the charts and the alert thresholds once the tab is opened', async () => {
    const wrapper = mountDetail()
    await settle(wrapper)

    await resourcesTab(wrapper)!.trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="resource-charts"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="resource-alerts"]').exists()).toBe(true)
  })

  it('gates the tab content behind the resource_history feature', async () => {
    const wrapper = mountDetail()
    await settle(wrapper)

    await resourcesTab(wrapper)!.trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-feature="resource_history"]').exists()).toBe(true)
  })

  it('leaves the Details tab intact', async () => {
    const wrapper = mountDetail()
    await settle(wrapper)

    expect(wrapper.text()).toContain('External ID')

    await resourcesTab(wrapper)!.trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).not.toContain('External ID')

    const detailsTab = wrapper.findAll('button').find((b) => b.text().includes('Details'))
    await detailsTab!.trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('External ID')
  })
})
