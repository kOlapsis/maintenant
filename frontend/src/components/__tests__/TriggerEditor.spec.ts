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

  it('offers every source an alert can carry', () => {
    const wrapper = mount(TriggerEditor, {
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    const chips = wrapper.findAll('button').map((b) => b.text())
    expect(chips).toEqual(
      expect.arrayContaining([
        'container', 'endpoint', 'heartbeat', 'certificate', 'resource', 'update',
        'security', 'agent', 'host', 'swarm', 'kubernetes',
      ]),
    )
    expect(chips).not.toContain('monitor')
  })

  it('sends back untouched a stored source it does not know', async () => {
    const wrapper = mount(TriggerEditor, {
      props: { trigger: { ...trigger, filter_sources: 'monitor' } },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    expect(wrapper.text()).toContain('monitor (unknown)')
    await wrapper.findAll('button').find((b) => b.text().includes('Save changes'))!.trigger('click')
    await flushPromises()

    expect(update.mock.calls[0]![1].filter_sources).toBe('monitor')
  })

  it('removes an unknown source only when the user removes it', async () => {
    const wrapper = mount(TriggerEditor, {
      props: { trigger: { ...trigger, filter_sources: 'monitor,container' } },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    await wrapper.find('button[aria-label="Remove monitor"]').trigger('click')
    expect(wrapper.text()).not.toContain('monitor')
    await wrapper.findAll('button').find((b) => b.text().includes('Save changes'))!.trigger('click')
    await flushPromises()

    expect(update.mock.calls[0]![1].filter_sources).toBe('container')
  })

  it('tells how to scope the entities that carry no id', () => {
    const wrapper = mount(TriggerEditor, {
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    expect(wrapper.text()).toContain('infrastructure:')
    expect(wrapper.text()).toContain('swarm_cluster:')
  })

  it('shows a scope example shaped like a real entity id', () => {
    const wrapper = mount(TriggerEditor, {
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    expect(wrapper.find('input[placeholder^="container:"]').attributes('placeholder')).toMatch(
      /^container:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    )
  })
})
