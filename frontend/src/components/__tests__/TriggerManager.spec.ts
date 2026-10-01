// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import type { AlertTrigger } from '@/types/triggers'
import { ApiError } from '@/services/apiFetch'
import TriggerManager from '@/components/TriggerManager.vue'
import TriggerList from '@/components/TriggerList.vue'

const trigger: AlertTrigger = {
  id: 't1',
  name: 'Scoped',
  filter_severities: 'critical',
  filter_sources: '',
  filter_scopes: 'container:42',
  enabled: true,
  notify_on_resolve: false,
  channel_ids: ['c1'],
  created_at: '',
  updated_at: '',
}

const update = vi.fn()

vi.mock('@/stores/triggers', () => ({
  useTriggersStore: () => ({
    triggers: [trigger],
    loading: false,
    error: null,
    fetchTriggers: vi.fn(),
    update,
    remove: vi.fn(),
    triggersForChannel: () => [],
  }),
}))

vi.mock('@/stores/channels', () => ({
  useChannelsStore: () => ({ channels: [], fetchChannels: vi.fn() }),
}))

vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => vi.fn() }))

function mountManager() {
  return mount(TriggerManager, { global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('TriggerManager', () => {
  beforeEach(() => {
    update.mockReset()
  })

  it('switches a trigger off without touching its filters', async () => {
    update.mockResolvedValue({ ...trigger, enabled: false })
    const wrapper = mountManager()
    await flushPromises()

    wrapper.findComponent(TriggerList).vm.$emit('toggle', trigger)
    await flushPromises()

    expect(update).toHaveBeenCalledWith('t1', {
      name: 'Scoped',
      filter_severities: 'critical',
      filter_sources: '',
      filter_scopes: 'container:42',
      enabled: false,
      notify_on_resolve: false,
      channel_ids: ['c1'],
    })
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('shows a refused toggle instead of swallowing it', async () => {
    update.mockRejectedValue(
      new ApiError(
        403,
        { code: 'EDITION_REQUIRED', message: 'This feature requires the Personal edition.' },
        'This feature requires the Personal edition.',
      ),
    )
    const wrapper = mountManager()
    await flushPromises()

    wrapper.findComponent(TriggerList).vm.$emit('toggle', trigger)
    await flushPromises()

    const alert = wrapper.find('[role="alert"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toBe('This feature requires the Personal edition.')
  })
})
