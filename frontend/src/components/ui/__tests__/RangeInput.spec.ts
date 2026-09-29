// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RangeInput from '@/components/ui/RangeInput.vue'

describe('RangeInput', () => {
  it('renders a native range input with the given bounds', () => {
    const wrapper = mount(RangeInput, { props: { label: 'CPU threshold', min: 1, max: 100, modelValue: 90 } })
    const input = wrapper.find('input[type="range"]')
    expect(input.attributes('min')).toBe('1')
    expect(input.attributes('max')).toBe('100')
    expect(input.attributes('aria-label')).toBe('CPU threshold')
  })

  it('emits update:modelValue as a number on input', async () => {
    const wrapper = mount(RangeInput, { props: { label: 'CPU threshold', min: 1, max: 100, modelValue: 50 } })
    const input = wrapper.find('input')
    await input.setValue('75')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([75])
  })

  it('derives the gradient fill from the current value', () => {
    const wrapper = mount(RangeInput, { props: { label: 'CPU threshold', min: 0, max: 100, modelValue: 25 } })
    expect((wrapper.find('input').element as HTMLElement).style.getPropertyValue('--fill')).toBe('25%')
  })
})
