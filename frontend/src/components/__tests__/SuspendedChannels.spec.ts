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
import { mount, RouterLinkStub } from '@vue/test-utils'
import { ref, computed } from 'vue'
import type { SuspendedChannel } from '@/services/editionApi'
import type { NotificationChannel } from '@/services/alertApi'
import SuspendedChannelsBanner from '@/components/SuspendedChannelsBanner.vue'
import ChannelManager from '@/components/ChannelManager.vue'

const suspended = ref<SuspendedChannel[]>([])
const channels = ref<NotificationChannel[]>([])

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ suspendedChannels: computed(() => suspended.value) }),
}))

vi.mock('@/stores/channels', () => ({
  useChannelsStore: () => ({
    get channels() {
      return channels.value
    },
    channelsLoading: false,
    fetchChannels: vi.fn(),
  }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => vi.fn() }))

function channel(over: Partial<NotificationChannel>): NotificationChannel {
  return {
    id: 'c1',
    name: 'oncall',
    type: 'slack',
    url: 'https://hooks.example.com/x',
    headers: '',
    enabled: true,
    health: 'healthy',
    created_at: '',
    updated_at: '',
    ...over,
  }
}

describe('SuspendedChannelsBanner', () => {
  beforeEach(() => {
    suspended.value = []
  })

  it('stays hidden while no channel is suspended', () => {
    const wrapper = mount(SuspendedChannelsBanner, { global: { stubs: { RouterLink: RouterLinkStub } } })
    expect(wrapper.find('[data-test="suspended-channels-banner"]').exists()).toBe(false)
  })

  it('names every suspended channel, the edition it needs, and links to channels and editions', () => {
    suspended.value = [
      { id: 'a', name: 'ops-slack', type: 'slack', required_edition: 'pro' },
      { id: 'b', name: 'ops-mail', type: 'email', required_edition: 'personal' },
    ]
    const wrapper = mount(SuspendedChannelsBanner, { global: { stubs: { RouterLink: RouterLinkStub } } })

    const banner = wrapper.find('[data-test="suspended-channels-banner"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('2 notification channels suspended')
    expect(banner.text()).toContain('ops-slack (slack, requires Pro)')
    expect(banner.text()).toContain('ops-mail (email, requires Personal)')
    expect(banner.attributes('role')).toBe('alert')

    const targets = wrapper.findAllComponents(RouterLinkStub).map((l) => l.props('to'))
    expect(targets).toEqual(['/channels', '/editions'])
  })

  it('uses the singular for one channel', () => {
    suspended.value = [{ id: 'a', name: 'ops-slack', type: 'slack', required_edition: 'pro' }]
    const wrapper = mount(SuspendedChannelsBanner, { global: { stubs: { RouterLink: RouterLinkStub } } })
    expect(wrapper.text()).toContain('1 notification channel suspended')
  })
})

describe('ChannelManager suspended badge', () => {
  it('marks a suspended channel with the edition it requires, and only that one', () => {
    channels.value = [
      channel({ id: 'c1', name: 'ops-slack', type: 'slack', suspended: true, required_edition: 'pro' }),
      channel({ id: 'c2', name: 'ops-hook', type: 'webhook', suspended: false }),
    ]
    const wrapper = mount(ChannelManager, { global: { stubs: { ChannelWizard: true } } })

    const badges = wrapper.findAll('[data-test="channel-suspended"]')
    expect(badges).toHaveLength(1)
    expect(badges[0]!.text()).toBe('Suspended · requires Pro')
  })
})
