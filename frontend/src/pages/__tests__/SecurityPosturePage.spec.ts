// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SecurityPosturePage from '@/pages/SecurityPosturePage.vue'

vi.mock('@/commercial/stores/posture', () => ({
  usePostureStore: () => ({ posture: null }),
}))

vi.mock('@/stores/containers', () => ({
  useContainersStore: () => ({ runtimeLabel: 'Docker' }),
}))

describe('SecurityPosturePage help', () => {
  beforeEach(() => localStorage.clear())

  it('names the weighted categories the scorer uses', async () => {
    const wrapper = mount(SecurityPosturePage, {
      global: { stubs: { FeatureGate: true, SecurityPosturePanel: true, UnlockCta: true } },
    })
    await flushPromises()

    const help = wrapper.text()
    for (const category of [
      'vulnerabilities (CVEs, 30%)',
      'network exposure (25%',
      'TLS certificates (20%)',
      'available updates (15%)',
      'image age (10%)',
    ]) {
      expect(help).toContain(category)
    }
    expect(help).not.toContain('configuration risks')
  })
})
