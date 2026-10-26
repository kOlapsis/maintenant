// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import HeartbeatDetail from '@/components/HeartbeatDetail.vue'
import { getHeartbeat, listExecutions, rotatePingToken, type Heartbeat } from '@/services/heartbeatApi'

// 043-public-surfaces-hardening, US2 scenarios 1 and 3, FR-013: the detail panel shows the token URL and rotates it.

vi.mock('@/services/heartbeatApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/heartbeatApi')>()
  return { ...actual, getHeartbeat: vi.fn(), listExecutions: vi.fn(), rotatePingToken: vi.fn() }
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

describe('HeartbeatDetail', () => {
  beforeEach(() => {
    vi.mocked(getHeartbeat).mockReset().mockResolvedValue({ heartbeat })
    vi.mocked(listExecutions).mockReset().mockResolvedValue({ executions: [], total: 0 })
    vi.mocked(rotatePingToken).mockReset()
  })

  it('builds the ping URL from the token, not from the id', async () => {
    const wrapper = mount(HeartbeatDetail, { props: { heartbeatId: heartbeat.id } })
    await flushPromises()

    expect(wrapper.text()).toContain(`/ping/${heartbeat.ping_token}`)
    expect(wrapper.text()).not.toContain(`/ping/${heartbeat.id}`)
  })

  it('rotates the token from the panel and shows the new URL', async () => {
    const rotated: Heartbeat = { ...heartbeat, ping_token: '9c2fcccc-0000-4000-8000-000000000003' }
    vi.mocked(rotatePingToken).mockResolvedValue(rotated)

    const wrapper = mount(HeartbeatDetail, { props: { heartbeatId: heartbeat.id } })
    await flushPromises()

    const button = wrapper.findAll('button').find((b) => /rotate/i.test(b.text()))
    expect(button, 'the panel offers a button to rotate the token').toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(rotatePingToken).toHaveBeenCalledTimes(1)
    expect(rotatePingToken).toHaveBeenCalledWith(heartbeat.id)
    expect(wrapper.text()).toContain(`/ping/${rotated.ping_token}`)
    expect(wrapper.text()).not.toContain(heartbeat.ping_token)
  })
})
