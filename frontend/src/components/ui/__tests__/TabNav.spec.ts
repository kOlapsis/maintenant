// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TabNav, { type TabNavItem } from '@/components/ui/TabNav.vue'

const items: TabNavItem[] = [
  { value: 'history', label: 'History' },
  { value: 'triggers', label: 'Triggers', count: 4 },
  { value: 'silence', label: 'Silence', count: 2, countTone: 'warn' },
]

describe('TabNav', () => {
  it('marks the active tab with aria-selected', () => {
    const wrapper = mount(TabNav, { props: { items, ariaLabel: 'Demo tabs', modelValue: 'triggers' } })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs[0]?.attributes('aria-selected')).toBe('false')
    expect(tabs[1]?.attributes('aria-selected')).toBe('true')
  })

  it('renders counts via CountBadge', () => {
    const wrapper = mount(TabNav, { props: { items, ariaLabel: 'Demo tabs', modelValue: 'history' } })
    expect(wrapper.findAll('[role="tab"]')[1]?.text()).toContain('4')
  })

  it('updates the model on click', async () => {
    const wrapper = mount(TabNav, { props: { items, ariaLabel: 'Demo tabs', modelValue: 'history' } })
    await wrapper.findAll('[role="tab"]')[2]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['silence'])
  })

  it('moves selection with ArrowRight/ArrowLeft, wrapping at the ends', async () => {
    const wrapper = mount(TabNav, { props: { items, ariaLabel: 'Demo tabs', modelValue: 'silence' } })
    const tabs = wrapper.findAll('[role="tab"]')
    await tabs[2]?.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['history'])

    await tabs[0]?.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual(['silence'])
  })
})
