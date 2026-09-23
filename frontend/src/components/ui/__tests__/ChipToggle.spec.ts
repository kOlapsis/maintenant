// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChipToggle from '@/components/ui/ChipToggle.vue'

describe('ChipToggle', () => {
  it('reflects pressed in aria-pressed', () => {
    const wrapper = mount(ChipToggle, { props: { pressed: true }, slots: { default: 'critical' } })
    expect(wrapper.find('button').attributes('aria-pressed')).toBe('true')
  })

  it('emits toggle on click', async () => {
    const wrapper = mount(ChipToggle, { props: { pressed: false }, slots: { default: 'critical' } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('toggle')).toHaveLength(1)
  })

  it('does not emit toggle when disabled', async () => {
    const wrapper = mount(ChipToggle, { props: { pressed: false, disabled: true }, slots: { default: 'critical' } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('toggle')).toBeUndefined()
  })

  it('renders a removable chip with an accessible remove button instead of a toggle', () => {
    const wrapper = mount(ChipToggle, {
      props: { removable: true, removeLabel: 'Remove container:42' },
      slots: { default: 'container:42' },
    })
    expect(wrapper.find('button[aria-pressed]').exists()).toBe(false)
    const removeButton = wrapper.find('button[aria-label="Remove container:42"]')
    expect(removeButton.exists()).toBe(true)
  })

  it('emits remove when the remove button is clicked', async () => {
    const wrapper = mount(ChipToggle, { props: { removable: true }, slots: { default: 'container:42' } })
    await wrapper.find('button[aria-label="Remove"]').trigger('click')
    expect(wrapper.emitted('remove')).toHaveLength(1)
  })
})
