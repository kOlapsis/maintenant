// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import FormField from '@/components/ui/FormField.vue'

interface SlotProps {
  id: string
  describedBy?: string
  invalid: boolean
}

describe('FormField', () => {
  it('wires the label for and the slot id together', () => {
    const wrapper = mount(FormField, {
      props: { label: 'Name' },
      slots: { default: (props: SlotProps) => h('input', { id: props.id }) },
    })
    const labelFor = wrapper.find('label').attributes('for')
    expect(labelFor).toBeTruthy()
    expect(wrapper.find('input').attributes('id')).toBe(labelFor)
  })

  it('uses the given id instead of generating one', () => {
    const wrapper = mount(FormField, {
      props: { label: 'Name', id: 'custom-id' },
      slots: { default: (props: SlotProps) => h('input', { id: props.id }) },
    })
    expect(wrapper.find('label').attributes('for')).toBe('custom-id')
    expect(wrapper.find('input').attributes('id')).toBe('custom-id')
  })

  it('exposes describedBy pointing at the hint when there is no error', () => {
    const wrapper = mount(FormField, {
      props: { label: 'Name', hint: 'Pick something short' },
      slots: { default: (props: SlotProps) => h('input', { 'aria-describedby': props.describedBy }) },
    })
    const describedBy = wrapper.find('input').attributes('aria-describedby')
    expect(describedBy).toBeTruthy()
    expect(wrapper.find(`#${describedBy}`).text()).toBe('Pick something short')
  })

  it('shows the error instead of the hint, and marks the field invalid', () => {
    const wrapper = mount(FormField, {
      props: { label: 'Name', hint: 'Pick something short', error: 'Required' },
      slots: { default: (props: SlotProps) => h('input', { 'aria-invalid': props.invalid }) },
    })
    expect(wrapper.text()).toContain('Required')
    expect(wrapper.text()).not.toContain('Pick something short')
    expect(wrapper.find('input').attributes('aria-invalid')).toBe('true')
  })
})
