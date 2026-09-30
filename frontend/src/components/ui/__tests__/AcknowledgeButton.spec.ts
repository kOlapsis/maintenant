// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { Alert } from '@/services/alertApi'
import AcknowledgeButton from '@/components/ui/AcknowledgeButton.vue'

let release: () => void = () => {}
const acknowledgeAlert = vi.fn(() => new Promise<void>((resolve) => (release = resolve)))

vi.mock('@/stores/alerts', () => ({
  useAlertsStore: () => ({ acknowledgeAlert }),
}))

function alertWith(over: Partial<Alert>): Alert {
  return { id: 'a1', status: 'active', ...over } as Alert
}

describe('AcknowledgeButton', () => {
  it('offers to acknowledge, then says it is doing so', async () => {
    const wrapper = mount(AcknowledgeButton, { props: { alert: alertWith({}) } })
    const button = wrapper.find('button')
    expect(button.text()).toBe('Acknowledge')

    await button.trigger('click')
    expect(button.text()).toBe('Acknowledging…')

    release()
    await flushPromises()
    expect(button.text()).toBe('Acknowledge')
  })

  it('names who acknowledged the alert', () => {
    const wrapper = mount(AcknowledgeButton, {
      props: { alert: alertWith({ acknowledged_at: '2026-09-30T12:00:00Z', acknowledged_by: 'alice' }) },
    })
    const done = wrapper.find('.ack-done')
    expect(done.text()).toBe('Acknowledged')
    expect(done.attributes('title')).toBe('Acknowledged by alice')
  })

  it('says acknowledged when nobody is named', () => {
    const wrapper = mount(AcknowledgeButton, {
      props: { alert: alertWith({ acknowledged_at: '2026-09-30T12:00:00Z' }) },
    })
    expect(wrapper.find('.ack-done').attributes('title')).toBe('Acknowledged')
  })
})
