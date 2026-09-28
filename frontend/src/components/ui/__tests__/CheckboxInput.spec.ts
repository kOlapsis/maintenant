// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'

describe('CheckboxInput', () => {
  it('round-trips a boolean model', async () => {
    const wrapper = mount(CheckboxInput, { props: { label: 'Accept', modelValue: false } })
    expect(wrapper.find('input').element.checked).toBe(false)
    await wrapper.find('input').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('adds its value to an array model when checked', async () => {
    const wrapper = mount(CheckboxInput, { props: { label: 'One', value: 'one', modelValue: [] } })
    await wrapper.find('input').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['one']])
  })

  it('removes its value from an array model when unchecked', async () => {
    const wrapper = mount(CheckboxInput, { props: { label: 'One', value: 'one', modelValue: ['one', 'two'] } })
    expect(wrapper.find('input').element.checked).toBe(true)
    await wrapper.find('input').setValue(false)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['two']])
  })
})
