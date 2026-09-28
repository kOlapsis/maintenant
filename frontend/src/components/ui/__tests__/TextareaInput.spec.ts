// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TextareaInput from '@/components/ui/TextareaInput.vue'

describe('TextareaInput', () => {
  it('round-trips the string model', async () => {
    const wrapper = mount(TextareaInput, { props: { modelValue: 'hello' } })
    expect(wrapper.find('textarea').element.value).toBe('hello')
    await wrapper.find('textarea').setValue('world')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['world'])
  })

  it('defaults to 3 rows and accepts an override', () => {
    const wrapper = mount(TextareaInput, { props: { modelValue: '' } })
    expect(wrapper.find('textarea').attributes('rows')).toBe('3')
    const custom = mount(TextareaInput, { props: { modelValue: '', rows: 6 } })
    expect(custom.find('textarea').attributes('rows')).toBe('6')
  })

  it('marks the field invalid', () => {
    const wrapper = mount(TextareaInput, { props: { modelValue: '', invalid: true } })
    expect(wrapper.find('textarea').attributes('aria-invalid')).toBe('true')
  })
})
