// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import TopConsumersWidget from '@/components/TopConsumersWidget.vue'
import { useEdition } from '@/composables/useEdition'
import { editionLinks, editionPayload } from './communityLimitFixtures'

// 051-community-label-limits, US4 scenarios 4 and 5, FR-015: past the 7-day
// history of Community, the widget names the edition that goes further and
// leads to the Editions page; Personal sees the widget as today.

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))

function mountWidget() {
  return mount(TopConsumersWidget, {
    props: { metric: 'cpu' as const, period: '7d', consumers: [] },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('TopConsumersWidget beyond the history cap', () => {
  it('on Community, a window past 7 days names Personal and links to the Editions page', async () => {
    useEdition().edition.value = editionPayload('community')
    const wrapper = mountWidget()

    await wrapper.find('[data-test="period-30d"]').trigger('click')

    expect(wrapper.emitted('update:period')).toBeUndefined()
    expect(editionLinks(wrapper)).toBeGreaterThan(0)
    expect(wrapper.text()).toContain('Personal')
  })

  it('on Personal, nothing changes: no edition message', async () => {
    useEdition().edition.value = editionPayload('personal')
    const wrapper = mountWidget()

    await wrapper.find('[data-test="period-90d"]').trigger('click')

    expect(editionLinks(wrapper)).toBe(0)
    expect(wrapper.text()).not.toContain('Personal')
  })
})
