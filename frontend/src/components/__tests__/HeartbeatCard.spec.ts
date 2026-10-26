// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import HeartbeatCard from '@/components/HeartbeatCard.vue'
import type { Heartbeat } from '@/services/heartbeatApi'

// 043-public-surfaces-hardening, US2 scenario 1, FR-013: the interface builds the ping URL from the token.

vi.mock('@/services/uptimeApi', () => ({
  fetchHeartbeatDailyUptime: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/services/heartbeatApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/heartbeatApi')>()
  return { ...actual, pauseHeartbeat: vi.fn(), resumeHeartbeat: vi.fn(), deleteHeartbeat: vi.fn() }
})

vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => async () => true,
}))

const heartbeat: Heartbeat = {
  id: '0199aaaa-0000-7000-8000-000000000001',
  ping_token: '4f1ebbbb-0000-4000-8000-000000000002',
  name: 'nightly backup',
  status: 'up',
  alert_state: 'normal',
  interval_seconds: 3600,
  grace_seconds: 300,
  consecutive_failures: 0,
  consecutive_successes: 1,
  created_at: '2026-10-21T00:00:00Z',
  updated_at: '2026-10-21T00:00:00Z',
}

describe('HeartbeatCard', () => {
  it('shows and copies the ping URL built from the token, not from the id', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    const wrapper = mount(HeartbeatCard, { props: { heartbeat } })
    await flushPromises()

    expect(wrapper.text()).toContain('/ping/4f1ebbbb')
    expect(wrapper.text()).not.toContain('0199aaaa')

    const copy = wrapper.findAll('button').find((b) => /copy/i.test(b.text()))
    expect(copy, 'the card offers to copy the ping URL').toBeDefined()
    await copy!.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/ping/${heartbeat.ping_token}`)
  })
})
