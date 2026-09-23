// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

describe('ToggleSwitch', () => {
  it('reflects the model in aria-checked', () => {
    const wrapper = mount(ToggleSwitch, { props: { label: 'Enabled', modelValue: true } })
    expect(wrapper.find('[role="switch"]').attributes('aria-checked')).toBe('true')
  })

  it('toggles the model on click', async () => {
    const wrapper = mount(ToggleSwitch, { props: { label: 'Enabled', modelValue: false } })
    await wrapper.find('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('uses the label as aria-label and hides it visually by default', () => {
    const wrapper = mount(ToggleSwitch, { props: { label: 'Enabled', modelValue: false } })
    expect(wrapper.find('[role="switch"]').attributes('aria-label')).toBe('Enabled')
    expect(wrapper.text()).not.toContain('Enabled')
  })

  it('shows the label when showLabel is set', () => {
    const wrapper = mount(ToggleSwitch, { props: { label: 'Enabled', modelValue: false, showLabel: true } })
    expect(wrapper.text()).toContain('Enabled')
  })

  it('does not toggle when disabled', async () => {
    const wrapper = mount(ToggleSwitch, { props: { label: 'Enabled', modelValue: false, disabled: true } })
    await wrapper.find('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
