import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import HeartbeatsPage from '@/pages/HeartbeatsPage.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'

const route = { query: {} as Record<string, string> }

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
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

describe('HeartbeatsPage', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('no longer renders the outgoing tab toggle or panel', () => {
    const wrapper = mountPage()
    expect(wrapper.findComponent({ name: 'SegmentedToggle' }).exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(false)
  })

  it('shows the incoming heartbeats subtitle only', () => {
    const wrapper = mountPage()
    expect(wrapper.text()).toContain('Passive cron & scheduled task monitoring')
    expect(wrapper.text()).not.toContain('Ping other Maintenant instances so they alert if this one goes down')
  })
})
