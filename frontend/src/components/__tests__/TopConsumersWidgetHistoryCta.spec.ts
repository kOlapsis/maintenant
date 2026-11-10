// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import TopConsumersWidget from '@/components/TopConsumersWidget.vue'
import { useEdition } from '@/composables/useEdition'
import { editionPayload } from './communityLimitFixtures'

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))

describe('TopConsumersWidget history message', () => {
  it('on Community, names Pro and Personal as the editions with a longer history', () => {
    useEdition().edition.value = editionPayload('community')
    const wrapper = mount(TopConsumersWidget, {
      props: { metric: 'cpu' as const, period: '7d', consumers: [] },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })

    expect(wrapper.findComponent(RouterLinkStub).text()).toBe(
      'Community keeps 7d of history. Pro and Personal go further.',
    )
  })
})
