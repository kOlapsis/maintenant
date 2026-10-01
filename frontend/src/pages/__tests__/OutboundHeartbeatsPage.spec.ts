import { describe, it, expect } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import OutboundHeartbeatsPage from '@/pages/OutboundHeartbeatsPage.vue'

describe('OutboundHeartbeatsPage', () => {
  it('mounts the outbound heartbeats panel', () => {
    const wrapper = shallowMount(OutboundHeartbeatsPage)
    expect(wrapper.findComponent({ name: 'OutboundHeartbeatsPanel' }).exists()).toBe(true)
  })
})
