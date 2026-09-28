import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { ref } from 'vue'
import OutboundHeartbeatsPage from '@/pages/OutboundHeartbeatsPage.vue'

const isDemo = ref(false)

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ isDemo }),
}))

function mountPage() {
  return shallowMount(OutboundHeartbeatsPage)
}

describe('OutboundHeartbeatsPage', () => {
  it('mounts the outbound heartbeats panel outside demo mode', () => {
    isDemo.value = false
    const wrapper = mountPage()
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'EmptyState' }).exists()).toBe(false)
  })

  it('does not mount the panel in demo mode and shows the unavailable state instead', () => {
    isDemo.value = true
    const wrapper = mountPage()
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'EmptyState' }).exists()).toBe(true)
  })
})
