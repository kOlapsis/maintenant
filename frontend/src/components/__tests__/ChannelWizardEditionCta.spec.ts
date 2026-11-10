// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import ChannelWizard from '@/components/ChannelWizard.vue'
import { useEdition } from '@/composables/useEdition'
import { editionLinks, editionPayload } from './communityLimitFixtures'

// 051-community-label-limits, US4 scenarios 2 and 5, FR-015: choosing a channel
// the edition does not open names Personal and leads to the Editions page;
// Personal sees the form, as today.

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))
vi.mock('@/services/alertApi', () => ({
  createChannel: vi.fn(async () => ({ id: 'c1' })),
  testChannel: vi.fn(async () => ({ status: 'delivered' })),
}))

function mountWizard() {
  return mount(ChannelWizard, { global: { stubs: { RouterLink: RouterLinkStub, SmtpNotConfigured: true } } })
}

async function choose(wrapper: ReturnType<typeof mountWizard>, label: string) {
  const card = wrapper.findAll('button').find((b) => b.text().includes(label))
  if (!card) throw new Error(`${label} is not offered`)
  await card.trigger('click')
}

describe('ChannelWizard: channels opened by Personal', () => {
  for (const label of ['Email', 'Telegram']) {
    it(`on Community, choosing ${label} names Personal and links to the Editions page`, async () => {
      useEdition().edition.value = editionPayload('community')
      const wrapper = mountWizard()

      await choose(wrapper, label)

      expect(editionLinks(wrapper)).toBeGreaterThan(0)
      expect(wrapper.text()).toContain('Personal')
    })

    it(`on Personal, choosing ${label} opens its form without any edition message`, async () => {
      useEdition().edition.value = editionPayload('personal')
      const wrapper = mountWizard()

      await choose(wrapper, label)

      expect(wrapper.text()).toContain('Channel Name')
      expect(editionLinks(wrapper)).toBe(0)
    })
  }
})
