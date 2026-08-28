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

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelWizard from '@/components/ChannelWizard.vue'

// The capability table the running edition opens. Telegram is Personal, like
// email; Slack and Teams are Pro.
const REQUIRED: Record<string, string> = {
  smtp: 'personal',
  telegram: 'personal',
  slack: 'pro',
  teams: 'pro',
}

let open: string[] = []

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    hasFeature: (f: string) => open.includes(f),
    editionPermits: (f: string) => open.includes(f),
    requiredEditionFor: (f: string) => REQUIRED[f] ?? null,
  }),
}))

const createChannel = vi.fn(async () => ({ id: 'c1' }))
vi.mock('@/services/alertApi', () => ({
  createChannel: (...args: unknown[]) => createChannel(...(args as [])),
  testChannel: vi.fn(async () => ({ status: 'delivered' })),
}))

function mountWizard() {
  return mount(ChannelWizard, { global: { stubs: { EditionBadge: true, SmtpNotConfigured: true } } })
}

/** The n-th input of the form, asserted present so the test fails on a missing
 * field rather than on an undefined value three lines later. */
async function fill(wrapper: ReturnType<typeof mountWizard>, index: number, value: string) {
  const input = wrapper.findAll('input')[index]
  if (!input) throw new Error(`input #${index} is missing`)
  await input.setValue(value)
}

async function pickTelegram() {
  const wrapper = mountWizard()
  const telegram = wrapper
    .findAll('button')
    .find((b) => b.text().includes('Telegram'))
  await telegram!.trigger('click')
  return wrapper
}

describe('ChannelWizard — Telegram', () => {
  beforeEach(() => {
    open = ['telegram', 'smtp']
    createChannel.mockClear()
  })

  // FR-001c: a Community instance must see the channel exists. Hiding it would
  // make the feature undiscoverable, which is not what gating is for.
  it('shows Telegram in every edition, marked with the edition it needs', () => {
    open = []
    const wrapper = mountWizard()

    const telegram = wrapper.findAll('button').find((b) => b.text().includes('Telegram'))
    expect(telegram).toBeDefined()
    expect(telegram!.classes()).toContain('cursor-not-allowed')
  })

  it('does not open the form for a type the edition does not permit', async () => {
    open = []
    const wrapper = mountWizard()

    const telegram = wrapper.findAll('button').find((b) => b.text().includes('Telegram'))
    await telegram!.trigger('click')

    expect(wrapper.text()).not.toContain('Bot Token')
  })

  // FR-003 and FR-017: no URL to type, a token and a chat id instead, and one
  // help line that says where to get them.
  it('asks for a token and a chat id, never a URL', async () => {
    const wrapper = await pickTelegram()

    expect(wrapper.text()).toContain('Chat ID')
    expect(wrapper.text()).toContain('Bot Token')
    expect(wrapper.text()).toContain('Topic ID (optional)')
    expect(wrapper.text()).not.toContain('Webhook URL')
    expect(wrapper.text()).toContain('@BotFather')

    expect(wrapper.find('input[type="url"]').exists()).toBe(false)
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
  })

  it('sends the token and the topic on create', async () => {
    const wrapper = await pickTelegram()

    await fill(wrapper, 0, 'oncall')
    await fill(wrapper, 1, '-1001234567890')
    await fill(wrapper, 2, '8123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx')
    await fill(wrapper, 3, '42')
    await wrapper.find('form').trigger('submit')

    expect(createChannel).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'telegram',
        url: '-1001234567890',
        secret: '8123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        config: { thread_id: '42' },
      }),
    )
  })

  it('omits the topic when the group has none', async () => {
    const wrapper = await pickTelegram()

    await fill(wrapper, 0, 'oncall')
    await fill(wrapper, 1, '-1001234567890')
    await fill(wrapper, 2, '8123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx')
    await wrapper.find('form').trigger('submit')

    expect(createChannel).toHaveBeenCalledWith(
      expect.objectContaining({ config: undefined }),
    )
  })

  // The other types must keep the field they always had.
  it('still asks for a URL on a webhook channel', async () => {
    const wrapper = mountWizard()
    const webhook = wrapper.findAll('button').find((b) => b.text().includes('Webhook'))
    await webhook!.trigger('click')

    expect(wrapper.text()).toContain('Webhook URL')
    expect(wrapper.text()).not.toContain('Bot Token')
  })
})
