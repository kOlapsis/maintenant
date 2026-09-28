// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import OptionCards, { type OptionCardItem } from '@/components/ui/OptionCards.vue'

const options: OptionCardItem[] = [
  { value: 'discord', label: 'Discord', description: 'Webhook' },
  { value: 'webhook', label: 'Webhook', description: 'HTTP POST' },
  { value: 'email', label: 'Email', description: 'SMTP', disabled: true },
]

describe('OptionCards', () => {
  it('marks the selected option with aria-checked', () => {
    const wrapper = mount(OptionCards, { props: { options, ariaLabel: 'Channel type', modelValue: 'webhook' } })
    const radios = wrapper.findAll('[role="radio"]')
    expect(radios[0]?.attributes('aria-checked')).toBe('false')
    expect(radios[1]?.attributes('aria-checked')).toBe('true')
  })

  it('updates the model on click', async () => {
    const wrapper = mount(OptionCards, { props: { options, ariaLabel: 'Channel type', modelValue: null } })
    await wrapper.findAll('[role="radio"]')[0]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['discord'])
  })

  it('does not select a disabled option', async () => {
    const wrapper = mount(OptionCards, { props: { options, ariaLabel: 'Channel type', modelValue: null } })
    await wrapper.findAll('[role="radio"]')[2]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('moves focus with arrow keys, wrapping at the ends', async () => {
    const wrapper = mount(OptionCards, {
      props: { options, ariaLabel: 'Channel type', modelValue: null },
      attachTo: document.body,
    })
    const radios = wrapper.findAll('[role="radio"]')
    ;(radios[0]!.element as HTMLElement).focus()
    await radios[0]!.trigger('keydown', { key: 'ArrowLeft' })
    expect(document.activeElement).toBe(radios[2]!.element)
    wrapper.unmount()
  })
})
