// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub, type DOMWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import type { NotificationChannel } from '@/services/alertApi'
import ChannelManager from '@/components/ChannelManager.vue'

// Acceptance tests of 044-label-command-safety, US3 scenario 6, FR-025: the edit form never sends
// the masked headers nor the truncated URL back to the server.

const channels = ref<NotificationChannel[]>([])

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

const updateChannel = vi.fn(async () => ({}))
vi.mock('@/services/alertApi', () => ({
  createChannel: vi.fn(async () => ({})),
  updateChannel: (...args: unknown[]) => updateChannel(...(args as [])),
  deleteChannel: vi.fn(async () => undefined),
  testChannel: vi.fn(async () => ({ status: 'delivered' })),
}))

function channel(over: Partial<NotificationChannel>): NotificationChannel {
  return {
    id: 'c1',
    name: 'ops',
    type: 'webhook',
    url: 'https://hooks.example.com/x',
    headers: '',
    enabled: true,
    health: 'healthy',
    created_at: '2026-10-21T10:00:00Z',
    updated_at: '2026-10-21T10:00:00Z',
    ...over,
  }
}

function mountManager() {
  return mount(ChannelManager, { global: { stubs: { ChannelWizard: true, RouterLink: RouterLinkStub } } })
}

async function openEdit(wrapper: ReturnType<typeof mountManager>) {
  const edit = wrapper.findAll('button').find((b) => b.text() === 'Edit')
  if (!edit) throw new Error('Edit button is missing')
  await edit.trigger('click')
  const form = wrapper.find('form')
  if (!form.exists()) throw new Error('the edit form did not open')
  return form
}

function inputOf(form: DOMWrapper<Element>, label: string): DOMWrapper<HTMLInputElement> {
  const field = form.findAll('label').find((l) => l.text().startsWith(label))
  if (!field) throw new Error(`field "${label}" is missing`)
  const input = form.find<HTMLInputElement>(`input[id="${field.attributes('for') ?? ''}"]`)
  if (!input.exists()) throw new Error(`input of "${label}" is missing`)
  return input
}

function lastPayload(): Record<string, unknown> {
  const calls = updateChannel.mock.calls as unknown as [string, Record<string, unknown>][]
  const call = calls[calls.length - 1]
  if (!call) throw new Error('updateChannel was not called')
  return call[1]
}

describe('ChannelManager edit form and masked secrets', () => {
  beforeEach(() => {
    updateChannel.mockClear()
  })

  it('leaves masked headers and a truncated URL empty, says they are on file, and does not send them back', async () => {
    channels.value = [
      channel({
        id: 'c1',
        name: 'ops-slack',
        type: 'slack',
        url: 'https://hooks.slack.com',
        has_url_secret: true,
        headers: '{"Authorization":"***"}',
        has_headers: true,
      }),
    ]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    const url = inputOf(form, 'Webhook URL')
    const headers = inputOf(form, 'Custom Headers')
    expect((url.element as HTMLInputElement).value).toBe('')
    expect((headers.element as HTMLInputElement).value).toBe('')
    expect(form.text() + ' ' + (headers.attributes('placeholder') ?? '')).toContain('Headers on file')
    expect(form.text() + ' ' + (url.attributes('placeholder') ?? '')).toContain('https://hooks.slack.com')
    expect(wrapper.html()).not.toContain('***')

    await inputOf(form, 'Name').setValue('ops-slack-2')
    await form.trigger('submit.prevent')
    await wrapper.vm.$nextTick()

    expect(updateChannel).toHaveBeenCalledTimes(1)
    const payload = lastPayload()
    expect(payload.name).toBe('ops-slack-2')
    expect(payload).not.toHaveProperty('headers')
    expect(payload).not.toHaveProperty('url')
  })

  it('sends new headers and a new URL when the operator types them', async () => {
    channels.value = [
      channel({
        id: 'c2',
        name: 'ops-discord',
        type: 'discord',
        url: 'https://discord.com',
        has_url_secret: true,
        headers: '{"X-Trace":"***"}',
        has_headers: true,
      }),
    ]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    await inputOf(form, 'Webhook URL').setValue('https://discord.com/api/webhooks/1/new-token')
    await inputOf(form, 'Custom Headers').setValue('{"X-Trace":"2"}')
    await form.trigger('submit.prevent')
    await wrapper.vm.$nextTick()

    const payload = lastPayload()
    expect(payload.url).toBe('https://discord.com/api/webhooks/1/new-token')
    expect(payload.headers).toBe('{"X-Trace":"2"}')
  })

  it('keeps a generic webhook URL in the form and sends it, without any headers key when none is on file', async () => {
    channels.value = [channel({ id: 'c3', name: 'hook', type: 'webhook', url: 'https://hooks.example.com/x', has_url_secret: false, has_headers: false })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    expect((inputOf(form, 'Webhook URL').element as HTMLInputElement).value).toBe('https://hooks.example.com/x')
    expect(form.text()).not.toContain('Headers on file')

    await form.trigger('submit.prevent')
    await wrapper.vm.$nextTick()

    const payload = lastPayload()
    expect(payload.url).toBe('https://hooks.example.com/x')
    expect(payload).not.toHaveProperty('headers')
  })
})
