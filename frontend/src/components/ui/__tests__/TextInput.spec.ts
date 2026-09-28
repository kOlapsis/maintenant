// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TextInput from '@/components/ui/TextInput.vue'

describe('TextInput', () => {
  it('round-trips a string value', async () => {
    const wrapper = mount(TextInput, { props: { modelValue: 'hello' } })
    expect(wrapper.find('input').element.value).toBe('hello')
    await wrapper.find('input').setValue('world')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['world'])
  })

  it('emits a number for type=number, and null when cleared', async () => {
    const wrapper = mount(TextInput, { props: { type: 'number', modelValue: 1 } })
    await wrapper.find('input').setValue('42')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([42])
    await wrapper.find('input').setValue('')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([null])
  })

  it('marks the field invalid and passes through attrs', () => {
    const wrapper = mount(TextInput, {
      props: { modelValue: '', invalid: true },
      attrs: { placeholder: 'Search', 'aria-describedby': 'hint-1' },
    })
    const input = wrapper.find('input')
    expect(input.attributes('aria-invalid')).toBe('true')
    expect(input.attributes('placeholder')).toBe('Search')
    expect(input.attributes('aria-describedby')).toBe('hint-1')
  })
})
