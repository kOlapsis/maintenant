// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UiButton from '@/components/ui/UiButton.vue'

describe('UiButton', () => {
  it('renders the default slot and defaults to type=button, variant=secondary', () => {
    const wrapper = mount(UiButton, { slots: { default: 'Save' } })
    const button = wrapper.find('button')
    expect(button.text()).toBe('Save')
    expect(button.attributes('type')).toBe('button')
    expect(button.classes()).toContain('border-mnt-default')
  })

  it('disables and shows a spinner while loading', () => {
    const wrapper = mount(UiButton, { props: { loading: true }, slots: { default: 'Save' } })
    const button = wrapper.find('button')
    expect(button.attributes('disabled')).toBeDefined()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
  })

  it('is disabled when the disabled prop is set', () => {
    const wrapper = mount(UiButton, { props: { disabled: true } })
    expect(wrapper.find('button').attributes('disabled')).toBeDefined()
  })

  it('passes through attrs such as title and aria-label', () => {
    const wrapper = mount(UiButton, { attrs: { title: 'Delete', 'aria-label': 'Delete item' } })
    const button = wrapper.find('button')
    expect(button.attributes('title')).toBe('Delete')
    expect(button.attributes('aria-label')).toBe('Delete item')
  })

  it('forwards a native click listener passed as an attr', async () => {
    const onClick = vi.fn()
    const wrapper = mount(UiButton, { attrs: { onClick }, slots: { default: 'Save' } })
    await wrapper.find('button').trigger('click')
    expect(onClick).toHaveBeenCalledOnce()
  })
})
