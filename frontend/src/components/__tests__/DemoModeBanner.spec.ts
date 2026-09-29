// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import DemoModeBanner from '@/components/DemoModeBanner.vue'

const isDemo = ref(false)

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ isDemo }),
}))

describe('DemoModeBanner', () => {
  it('renders nothing when the instance is not in demo mode', () => {
    isDemo.value = false
    const wrapper = mount(DemoModeBanner)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('renders a non-dismissible banner when the instance is in demo mode', () => {
    isDemo.value = true
    const wrapper = mount(DemoModeBanner)
    const banner = wrapper.find('[role="alert"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('Demo mode: read-only.')
    expect(wrapper.find('button[aria-label="Dismiss"]').exists()).toBe(false)
  })
})
