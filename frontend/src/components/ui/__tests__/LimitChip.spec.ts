// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import LimitChip from '@/components/ui/LimitChip.vue'
import { useEdition } from '@/composables/useEdition'
import { editionLinks, editionPayload, LIMIT_TEXT } from '@/components/__tests__/communityLimitFixtures'

// 051-community-label-limits, FR-006: the state of an element left out by the cap, and the way out.

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))

describe('LimitChip', () => {
  it('says the element is not monitored because of the Community limit, with a link to the Editions page', () => {
    useEdition().edition.value = editionPayload('community')
    const wrapper = mount(LimitChip, { global: { stubs: { RouterLink: RouterLinkStub } } })

    expect(wrapper.text()).toMatch(LIMIT_TEXT)
    expect(editionLinks(wrapper)).toBeGreaterThan(0)
  })
})
