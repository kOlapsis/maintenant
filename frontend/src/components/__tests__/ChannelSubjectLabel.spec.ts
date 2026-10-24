// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import type { NotificationChannel, ChannelConfig } from '@/services/alertApi'
import ChannelWizard from '@/components/ChannelWizard.vue'
import ChannelManager from '@/components/ChannelManager.vue'

// Acceptance tests of 049-email-subject-prefix: the subject label field of the channel wizard and edit form.

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    hasFeature: () => true,
    editionPermits: () => true,
    requiredEditionFor: () => null,
  }),
}))

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

const createChannel = vi.fn()
const updateChannel = vi.fn()
const testChannel = vi.fn()
vi.mock('@/services/alertApi', () => ({
  createChannel: (...args: unknown[]) => createChannel(...args),
  updateChannel: (...args: unknown[]) => updateChannel(...args),
  deleteChannel: vi.fn(async () => undefined),
  testChannel: (...args: unknown[]) => testChannel(...args),
}))

type Scope = VueWrapper | DOMWrapper<Element>

function fieldInput(scope: Scope, label: RegExp): DOMWrapper<HTMLInputElement> | undefined {
  const l = scope.findAll('label').find((x) => label.test(x.text()))
  if (!l) return undefined
  const input = scope.find<HTMLInputElement>(`input[id="${l.attributes('for') ?? ''}"]`)
  return input.exists() ? input : undefined
}

function requiredInput(scope: Scope, label: RegExp, what: string): DOMWrapper<HTMLInputElement> {
  const input = fieldInput(scope, label)
  expect(input, `the ${what} field`).toBeDefined()
  return input!
}

const subjectInput = (scope: Scope) => requiredInput(scope, /subject/i, 'subject label')

type Sent = { config?: ChannelConfig | null } & Record<string, unknown>

function sentBy(mock: ReturnType<typeof vi.fn>, argIndex: number): Sent {
  const call = mock.mock.calls[mock.mock.calls.length - 1]
  if (!call) throw new Error('the API was not called')
  return call[argIndex] as Sent
}

function mountWizard() {
  return mount(ChannelWizard, { global: { stubs: { EditionBadge: true, SmtpNotConfigured: true } } })
}

async function pickType(wrapper: VueWrapper, label: string) {
  const card = wrapper.findAll('button').find((b) => b.text().includes(label))
  if (!card) throw new Error(`the ${label} card is missing`)
  await card.trigger('click')
}

async function fillEmailWizard(wrapper: VueWrapper, label: string) {
  await pickType(wrapper, 'Email')
  await requiredInput(wrapper, /^Channel Name/, 'name').setValue('ops mail')
  await requiredInput(wrapper, /^Email Address/, 'address').setValue('ops@example.com')
  await subjectInput(wrapper).setValue(label)
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

function emailChannel(over: Partial<NotificationChannel> = {}): NotificationChannel {
  return {
    id: 'm1',
    name: 'ops mail',
    type: 'email',
    url: 'ops@example.com',
    headers: '',
    enabled: true,
    health: 'healthy',
    created_at: '2026-10-24T10:00:00Z',
    updated_at: '2026-10-24T10:00:00Z',
    ...over,
  }
}

function mountManager() {
  return mount(ChannelManager, { global: { stubs: { ChannelWizard: true, RouterLink: RouterLinkStub } } })
}

async function openEdit(wrapper: VueWrapper) {
  const edit = wrapper.findAll('button').find((b) => b.text() === 'Edit')
  if (!edit) throw new Error('Edit button is missing')
  await edit.trigger('click')
  const form = wrapper.find('form')
  if (!form.exists()) throw new Error('the edit form did not open')
  return form
}

async function save(wrapper: VueWrapper) {
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  createChannel.mockReset()
  updateChannel.mockReset()
  testChannel.mockReset()
  createChannel.mockResolvedValue({ id: 'c1' })
  updateChannel.mockResolvedValue({})
  testChannel.mockResolvedValue({ status: 'delivered', response_code: 200 })
})

describe('ChannelWizard subject label', () => {
  // FR-001, FR-011, edge "other channel types": the field exists for email, and only for email.
  it.each([
    ['Email', true],
    ['Webhook', false],
    ['Discord', false],
    ['Telegram', false],
    ['Slack', false],
    ['Teams', false],
  ])('FR-011: %s channel shows the subject label field: %s', async (type, shown) => {
    const wrapper = mountWizard()
    await pickType(wrapper, type)
    expect(fieldInput(wrapper, /subject/i) !== undefined).toBe(shown)
  })

  // US2 scenario 3, FR-001, FR-005: the label is saved with the channel, and the test goes to the saved channel.
  it.each(['staging', 'prod-paris'])('US2 sc.3, FR-005: saves "%s" with the new channel, then tests that channel', async (label) => {
    createChannel.mockResolvedValue({ id: `id-${label}` })
    const wrapper = mountWizard()

    await fillEmailWizard(wrapper, label)

    expect(createChannel).toHaveBeenCalledTimes(1)
    expect(sentBy(createChannel, 0)).toMatchObject({ type: 'email', url: 'ops@example.com', config: { subject_label: label } })

    const test = wrapper.findAll('button').find((b) => b.text().includes('Send Test Notification'))
    if (!test) throw new Error('the wizard did not reach its test step')
    await test.trigger('click')
    await flushPromises()
    expect(testChannel).toHaveBeenCalledWith(`id-${label}`)
  })

  // FR-003: a channel created without a label has none.
  it('FR-003: sends no label when the field is left empty', async () => {
    const wrapper = mountWizard()

    await fillEmailWizard(wrapper, '')

    expect(createChannel).toHaveBeenCalledTimes(1)
    expect(sentBy(createChannel, 0).config?.subject_label ?? '').toBe('')
  })

  // US3 scenario 1, FR-007: a refused label keeps the user on the form, with the limit in the message.
  it('US3 sc.1, FR-007: a label over 64 characters is refused with the limit, and the wizard stays on the form', async () => {
    createChannel.mockRejectedValue(new Error('subject_label must be at most 64 characters'))
    const wrapper = mountWizard()

    await fillEmailWizard(wrapper, 'a'.repeat(65))

    expect(wrapper.find('form').exists()).toBe(true)
    expect(wrapper.text()).toContain('64')
    expect(wrapper.text()).not.toContain('Send Test Notification')
  })
})

describe('ChannelManager subject label', () => {
  // US1 scenario 6: the edit form shows the label on file, empty when there is none.
  it.each([
    ['{"subject_label":"prod-paris"}', 'prod-paris'],
    ['{"subject_label":"préprod 東京"}', 'préprod 東京'],
    [undefined, ''],
    ['{}', ''],
  ])('US1 sc.6: config %s is shown as "%s"', async (config, shown) => {
    channels.value = [emailChannel({ config })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    expect(subjectInput(form).element.value).toBe(shown)
  })

  // FR-011: the edit form offers the field on email channels only.
  it.each([
    ['email', true],
    ['webhook', false],
    ['telegram', false],
    ['slack', false],
  ])('FR-011: a %s channel shows the subject label field: %s', async (type, shown) => {
    channels.value = [emailChannel({ type, url: type === 'email' ? 'ops@example.com' : 'https://hooks.example.com/x' })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    expect(fieldInput(form, /subject/i) !== undefined).toBe(shown)
  })

  // FR-001, US2 scenario 4: a new label is sent in config.
  it.each(['staging', 'prod-lyon'])('FR-001, US2 sc.4: sends the new label "%s"', async (label) => {
    channels.value = [emailChannel({ config: '{"subject_label":"prod-paris"}' })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    await subjectInput(form).setValue(label)
    await save(wrapper)

    expect(updateChannel).toHaveBeenCalledTimes(1)
    expect(updateChannel.mock.calls[0]?.[0]).toBe('m1')
    expect(sentBy(updateChannel, 1).config).toMatchObject({ subject_label: label })
  })

  // US1 scenario 5, contract api.md: clearing the field sends config, so the server clears the label.
  it('US1 sc.5: clearing the label sends config without it', async () => {
    channels.value = [emailChannel({ config: '{"subject_label":"prod-paris"}' })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    await subjectInput(form).setValue('')
    await save(wrapper)

    const sent = sentBy(updateChannel, 1)
    expect(sent.config, 'config must be sent, or the server keeps the old label').toBeDefined()
    expect(sent.config?.subject_label ?? '').toBe('')
  })

  // FR-009: saving without touching the label keeps it.
  it('FR-009: a rename keeps the label on file', async () => {
    channels.value = [emailChannel({ config: '{"subject_label":"prod-paris"}' })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)
    expect(subjectInput(form).element.value).toBe('prod-paris')

    await requiredInput(form, /^Name/, 'name').setValue('ops mail 2')
    await save(wrapper)

    const sent = sentBy(updateChannel, 1)
    expect(sent.name).toBe('ops mail 2')
    expect(sent.config === undefined || sent.config?.subject_label === 'prod-paris', JSON.stringify(sent)).toBe(true)
  })

  // US3 scenarios 1 and 2, FR-007: a refused label keeps the form open and says what to fix.
  it.each([
    ['a'.repeat(65), 'subject_label must be at most 64 characters', /64/],
    ['prod\tparis', 'subject_label must not contain control characters', /control/i],
  ])('US3, FR-007: "%s" is refused with a message that says why, and the form stays open', async (label, serverMessage, says) => {
    updateChannel.mockRejectedValue(new Error(serverMessage))
    channels.value = [emailChannel({ config: '{"subject_label":"prod-paris"}' })]
    const wrapper = mountManager()
    const form = await openEdit(wrapper)

    await subjectInput(form).setValue(label)
    await save(wrapper)

    expect(wrapper.find('form').exists()).toBe(true)
    expect(wrapper.find('form').text()).toMatch(says)
  })
})
