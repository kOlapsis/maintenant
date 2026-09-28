// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import HeartbeatsPage from '@/pages/HeartbeatsPage.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'

const isDemo = ref(false)
const route = { query: { tab: 'outgoing' } as Record<string, string> }

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    isDemo,
    reload: vi.fn(),
    getQuota: () => ref({ used: 0, limit: 0, isUnlimited: true, isAtLimit: false, nearLimit: false }),
  }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace: vi.fn() }),
}))

vi.mock('@/stores/heartbeats', () => ({
  useHeartbeatsStore: () => ({
    heartbeats: [],
    loading: false,
    error: null,
    fetchHeartbeats: vi.fn(),
    connectSSE: vi.fn(),
    disconnectSSE: vi.fn(),
  }),
}))

function mountPage() {
  return shallowMount(HeartbeatsPage, {
    global: {
      provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } },
      stubs: { 'router-link': true },
    },
  })
}

describe('HeartbeatsPage outgoing tab', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('offers the outgoing tab outside demo mode', () => {
    isDemo.value = false
    const wrapper = mountPage()
    expect(wrapper.findComponent({ name: 'SegmentedToggle' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(true)
  })

  it('hides the outgoing tab in demo mode, even when the URL asks for it', () => {
    isDemo.value = true
    const wrapper = mountPage()
    expect(wrapper.findComponent({ name: 'SegmentedToggle' }).exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(false)
  })
})
