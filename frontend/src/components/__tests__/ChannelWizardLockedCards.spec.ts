// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import ChannelWizard from '@/components/ChannelWizard.vue'
import { useEdition } from '@/composables/useEdition'
import { editionPayload, isEditionsTarget } from './communityLimitFixtures'

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))
vi.mock('@/services/alertApi', () => ({ createChannel: vi.fn(), testChannel: vi.fn() }))

describe('ChannelWizard locked cards', () => {
  it('each locked card names the edition that opens it, in a link kept out of the disabled button', () => {
    useEdition().edition.value = editionPayload('community')
    const wrapper = mount(ChannelWizard, {
      global: { stubs: { RouterLink: RouterLinkStub, SmtpNotConfigured: true } },
    })

    const links = wrapper
      .findAllComponents(RouterLinkStub)
      .filter((l) => isEditionsTarget(l.props('to')))
    expect(links.map((l) => l.text())).toEqual([
      'Available from Personal',
      'Available from Personal',
      'Available from Pro',
      'Available from Pro',
    ])
    for (const link of links) expect(link.element.closest('button')).toBeNull()
  })
})
