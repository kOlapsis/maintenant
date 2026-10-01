// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import type { AlertTrigger } from '@/types/triggers'
import TriggerEditor from '@/components/TriggerEditor.vue'

const update = vi.fn()

vi.mock('@/stores/triggers', () => ({
  useTriggersStore: () => ({ create: vi.fn(), update }),
}))

vi.mock('@/stores/channels', () => ({
  useChannelsStore: () => ({ channels: [{ id: 'c1', name: 'ops', type: 'webhook', enabled: true }] }),
}))

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ hasFeature: () => true, requiredEditionFor: () => 'pro' }),
}))

const trigger: AlertTrigger = {
  id: 't1',
  name: 'Critical containers',
  filter_severities: 'critical',
  filter_sources: 'container',
  filter_scopes: 'container:42',
  enabled: true,
  notify_on_resolve: true,
  channel_ids: ['c1'],
  created_at: '',
  updated_at: '',
}

describe('TriggerEditor', () => {
  beforeEach(() => {
    update.mockReset()
  })

  it('offers no tag filter', () => {
    const wrapper = mount(TriggerEditor, {
      props: { trigger },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    expect(wrapper.text()).not.toMatch(/tag/i)
  })

  it('saves the filters it shows and nothing else', async () => {
    const wrapper = mount(TriggerEditor, {
      props: { trigger },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    const save = wrapper.findAll('button').find((b) => b.text().includes('Save changes'))
    expect(save).toBeDefined()
    await save!.trigger('click')
    await flushPromises()

    expect(update).toHaveBeenCalledOnce()
    const [id, req] = update.mock.calls[0]!
    expect(id).toBe('t1')
    expect(req).toEqual({
      name: 'Critical containers',
      filter_severities: 'critical',
      filter_sources: 'container',
      filter_scopes: 'container:42',
      enabled: true,
      notify_on_resolve: true,
      channel_ids: ['c1'],
    })
  })
})
