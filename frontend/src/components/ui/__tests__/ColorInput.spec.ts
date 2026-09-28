// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ColorInput from '@/components/ui/ColorInput.vue'

describe('ColorInput', () => {
  it('renders a color swatch and a text field with the same value', () => {
    const wrapper = mount(ColorInput, { props: { modelValue: '#22C55E', label: 'Accent' } })
    expect(wrapper.find('input[type="color"]').element).toHaveProperty('value', '#22c55e')
    expect(wrapper.find('input[type="text"]').element).toHaveProperty('value', '#22C55E')
  })

  it('emits update:modelValue when the swatch changes', async () => {
    const wrapper = mount(ColorInput, { props: { modelValue: '#000000' } })
    const swatch = wrapper.find('input[type="color"]')
    await swatch.setValue('#ffffff')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['#ffffff'])
  })

  it('emits update:modelValue when the hex text field changes', async () => {
    const wrapper = mount(ColorInput, { props: { modelValue: '#000000' } })
    const text = wrapper.find('input[type="text"]')
    await text.setValue('#123ABC')
    await text.trigger('change')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['#123ABC'])
  })
})
