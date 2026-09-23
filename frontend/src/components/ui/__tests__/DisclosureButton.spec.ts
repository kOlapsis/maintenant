// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DisclosureButton from '@/components/ui/DisclosureButton.vue'

describe('DisclosureButton', () => {
  it('reflects expanded state in aria-expanded', () => {
    const wrapper = mount(DisclosureButton, { props: { expanded: true }, slots: { default: 'Group' } })
    expect(wrapper.find('button').attributes('aria-expanded')).toBe('true')
  })

  it('sets aria-controls from controlsId', () => {
    const wrapper = mount(DisclosureButton, {
      props: { expanded: false, controlsId: 'panel-1' },
      slots: { default: 'Group' },
    })
    expect(wrapper.find('button').attributes('aria-controls')).toBe('panel-1')
  })

  it('emits toggle on click', async () => {
    const wrapper = mount(DisclosureButton, { props: { expanded: false }, slots: { default: 'Group' } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('toggle')).toHaveLength(1)
  })

  it('places the chevron after the content when chevronPosition is end', () => {
    const wrapper = mount(DisclosureButton, {
      props: { expanded: false, chevronPosition: 'end' },
      slots: { default: '<span class="label">Component</span>' },
    })
    const button = wrapper.find('button')
    const label = button.find('.label').element
    const chevron = button.find('svg').element
    expect(label.compareDocumentPosition(chevron) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
