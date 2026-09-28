// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CountBadge from '@/components/ui/CountBadge.vue'

describe('CountBadge', () => {
  it('renders the value', () => {
    const wrapper = mount(CountBadge, { props: { value: 7 } })
    expect(wrapper.text()).toBe('7')
  })

  it('defaults to the neutral tone', () => {
    const wrapper = mount(CountBadge, { props: { value: 1 } })
    expect(wrapper.classes()).toContain('bg-mnt-elevated')
  })

  it('applies the danger tone', () => {
    const wrapper = mount(CountBadge, { props: { value: 3, tone: 'danger' } })
    expect(wrapper.classes()).toContain('bg-mnt-status-down')
    expect(wrapper.classes()).toContain('text-mnt-status-down')
  })
})
