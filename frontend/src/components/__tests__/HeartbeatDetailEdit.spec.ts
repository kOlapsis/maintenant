// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import HeartbeatDetail from '@/components/HeartbeatDetail.vue'
import HeartbeatForm from '@/components/HeartbeatForm.vue'
import { ApiError } from '@/services/apiFetch'
import {
  getHeartbeat,
  listExecutions,
  rotatePingToken,
  updateHeartbeat,
  type CreateHeartbeatInput,
  type Heartbeat,
} from '@/services/heartbeatApi'
import { formatInterval } from '@/utils/scheduleFormat'
import { formatDeadline } from '@/utils/heartbeatFormat'

// 048-heartbeat-edit: US1 scenarios 1, 2 and 5, US3 scenarios 2 and 4, FR-001, FR-002, FR-008, FR-010, SC-003,
// edge cases "deleted in another tab", "remote agent", "rotation stays separate".

vi.mock('@/services/heartbeatApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/heartbeatApi')>()
  return { ...actual, getHeartbeat: vi.fn(), listExecutions: vi.fn(), rotatePingToken: vi.fn(), updateHeartbeat: vi.fn() }
})

vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => async () => true,
}))

const NOW = Date.UTC(2026, 9, 24, 12, 0, 0)
const at = (seconds: number) => new Date(NOW + seconds * 1000).toISOString()

function heartbeat(overrides: Partial<Heartbeat>): Heartbeat {
  return {
    id: '0199aaaa-0000-7000-8000-000000000001',
    ping_token: '4f1ebbbb-0000-4000-8000-0000000000aa',
    name: 'nightly backup',
    status: 'up',
    alert_state: 'normal',
    interval_seconds: 86400,
    grace_seconds: 600,
    consecutive_failures: 0,
    consecutive_successes: 4,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

const cases: { what: string; before: Heartbeat; edit: CreateHeartbeatInput; deadline: string }[] = [
  {
    what: 'a local heartbeat',
    before: heartbeat({ last_ping_at: at(-3600), next_deadline_at: at(-3600 + 87000) }),
    edit: { name: 'db dump', interval_seconds: 3600, grace_seconds: 300 },
    deadline: at(-3600 + 3900),
  },
  {
    what: 'a heartbeat of a remote agent',
    before: heartbeat({
      id: '0199aaaa-0000-7000-8000-000000000002',
      ping_token: '4f1ebbbb-0000-4000-8000-0000000000bb',
      name: 'hourly sync',
      interval_seconds: 3600,
      grace_seconds: 60,
      agent_id: '0199cccc-0000-7000-8000-00000000000c',
      last_ping_at: at(-600),
      next_deadline_at: at(-600 + 3660),
    }),
    edit: { name: 'eu replication', interval_seconds: 21600, grace_seconds: 900 },
    deadline: at(-600 + 22500),
  },
]

async function mountDetail(hb: Heartbeat) {
  vi.mocked(getHeartbeat).mockResolvedValue({ heartbeat: hb })
  const wrapper = mount(HeartbeatDetail, { props: { heartbeatId: hb.id } })
  await flushPromises()
  return wrapper
}

async function openEdit(wrapper: VueWrapper) {
  const edit = wrapper.findAll('button').find((b) => /^\s*edit\b/i.test(b.text()))
  expect(edit, 'the detail offers an Edit button').toBeDefined()
  await edit!.trigger('click')
  await flushPromises()
  const form = wrapper.findComponent(HeartbeatForm)
  expect(form.exists(), 'Edit opens the heartbeat form').toBe(true)
  return form
}

function showsDeadline(text: string, iso: string): boolean {
  return text.includes(new Date(iso).toLocaleString()) || text.includes(formatDeadline(iso))
}

describe('HeartbeatDetail: edit', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(NOW)
    vi.mocked(getHeartbeat).mockReset()
    vi.mocked(listExecutions).mockReset().mockResolvedValue({ executions: [], total: 0 })
    vi.mocked(rotatePingToken).mockReset()
    vi.mocked(updateHeartbeat).mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it.each(cases)('US1 sc.1, FR-001: Edit on $what opens the form prefilled with its current values', async ({ before }) => {
    const wrapper = await mountDetail(before)

    const form = await openEdit(wrapper)

    expect(form.props('initial')).toMatchObject({
      name: before.name,
      interval_seconds: before.interval_seconds,
      grace_seconds: before.grace_seconds,
    })
  })

  it.each(cases)(
    'US1 sc.2, FR-010, SC-003: saving $what shows the new name, schedule and expected time at once',
    async ({ before, edit, deadline }) => {
      vi.mocked(updateHeartbeat).mockResolvedValue({ ...before, ...edit, next_deadline_at: deadline, updated_at: at(0) })
      const wrapper = await mountDetail(before)
      const form = await openEdit(wrapper)

      form.vm.$emit('submit', edit)
      await flushPromises()

      expect(updateHeartbeat).toHaveBeenCalledTimes(1)
      expect(updateHeartbeat).toHaveBeenCalledWith(before.id, edit)
      const text = wrapper.text()
      expect(text).toContain(edit.name)
      expect(text).not.toContain(before.name)
      expect(text).toContain(formatInterval(edit.interval_seconds))
      expect(text).not.toContain(formatInterval(before.interval_seconds))
      expect(showsDeadline(text, deadline), `the expected time ${deadline} is shown (page: "${text}")`).toBe(true)
      expect(rotatePingToken, 'the edit never rotates the token').not.toHaveBeenCalled()
      expect(text, 'the ping URL does not change').toContain(`/ping/${before.ping_token}`)
    },
  )

  it('US1 sc.5: cancel changes nothing', async () => {
    const before = cases[0]!.before
    const wrapper = await mountDetail(before)
    const form = await openEdit(wrapper)

    form.vm.$emit('cancel')
    await flushPromises()

    expect(updateHeartbeat).not.toHaveBeenCalled()
    expect(wrapper.findComponent(HeartbeatForm).exists(), 'cancel closes the form').toBe(false)
    expect(wrapper.text()).toContain(before.name)
    expect(wrapper.text()).toContain(formatInterval(before.interval_seconds))
  })

  it.each([
    'invalid input: interval must be 60-604800 seconds',
    'invalid input: grace must be 0-300 seconds',
  ])('US3 sc.2 and sc.4, FR-008: a refusal shows "%s", keeps the form and the old values', async (message) => {
    const { before, edit } = cases[0]!
    vi.mocked(updateHeartbeat).mockRejectedValue(new ApiError(400, { code: 'INVALID_INPUT', message }, message))
    const wrapper = await mountDetail(before)
    const form = await openEdit(wrapper)

    form.vm.$emit('submit', edit)
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain(message)
    expect(text).toContain(before.name)
    expect(text).not.toContain(edit.name)
    expect(wrapper.findComponent(HeartbeatForm).exists(), 'the form stays open with what was typed').toBe(true)
  })

  it('edge case: a heartbeat deleted in another tab fails with a message saying it no longer exists', async () => {
    const { before, edit } = cases[0]!
    vi.mocked(updateHeartbeat).mockRejectedValue(
      new ApiError(404, { code: 'NOT_FOUND', message: 'Heartbeat not found' }, 'Heartbeat not found'),
    )
    const wrapper = await mountDetail(before)
    const form = await openEdit(wrapper)

    form.vm.$emit('submit', edit)
    await flushPromises()

    expect(wrapper.text()).toMatch(/no longer exists?|not found|deleted/i)
    expect(wrapper.text()).not.toContain(edit.name)
  })
})
