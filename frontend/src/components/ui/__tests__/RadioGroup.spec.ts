// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioGroup from '@/components/ui/RadioGroup.vue'

const options = [
  { value: 'a', label: 'Option A' },
  { value: 'b', label: 'Option B', hint: 'The second one' },
]

describe('RadioGroup', () => {
  it('checks the radio matching the model', () => {
    const wrapper = mount(RadioGroup, { props: { options, modelValue: 'b' } })
    const inputs = wrapper.findAll('input[type="radio"]')
    const checked = (i: (typeof inputs)[number] | undefined) => (i?.element as HTMLInputElement).checked
    expect(checked(inputs[0])).toBe(false)
    expect(checked(inputs[1])).toBe(true)
  })

  it('shares one generated name across all radios', () => {
    const wrapper = mount(RadioGroup, { props: { options, modelValue: 'a' } })
    const names = wrapper.findAll('input[type="radio"]').map((i) => i.attributes('name'))
    expect(names[0]).toBeTruthy()
    expect(names[0]).toBe(names[1])
  })

  it('emits the selected value on change', async () => {
    const wrapper = mount(RadioGroup, { props: { options, modelValue: 'a' } })
    await wrapper.findAll('input[type="radio"]')[1]?.setValue()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['b'])
  })

  it('exposes the radiogroup role', () => {
    const wrapper = mount(RadioGroup, { props: { options, modelValue: 'a', ariaLabel: 'Pick one' } })
    expect(wrapper.find('[role="radiogroup"]').attributes('aria-label')).toBe('Pick one')
  })
})
