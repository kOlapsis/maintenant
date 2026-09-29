// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SelectInput from '@/components/ui/SelectInput.vue'

const options = [
  { value: 'a', label: 'Option A' },
  { value: 'b', label: 'Option B' },
  { value: 'c', label: 'Option C', disabled: true },
]

describe('SelectInput', () => {
  it('renders options and round-trips the model', async () => {
    const wrapper = mount(SelectInput, { props: { options, modelValue: 'a' } })
    expect(wrapper.findAll('option')).toHaveLength(3)
    await wrapper.find('select').setValue('b')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['b'])
  })

  it('marks disabled options', () => {
    const wrapper = mount(SelectInput, { props: { options, modelValue: 'a' } })
    expect(wrapper.findAll('option')[2]?.attributes('disabled')).toBeDefined()
  })

  it('marks the field invalid', () => {
    const wrapper = mount(SelectInput, { props: { options, modelValue: 'a', invalid: true } })
    expect(wrapper.find('select').attributes('aria-invalid')).toBe('true')
  })
})
