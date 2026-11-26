// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import FeatureGate from '@/components/FeatureGate.vue'

const isLoaded = ref(false)

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({ hasFeature: () => false, requiredEditionFor: () => null, isLoaded }),
}))

function mountGate() {
  return mount(FeatureGate, {
    props: { feature: 'incidents', title: 'Incidents' },
    slots: { default: 'the feature' },
    global: { stubs: { RouterLink: true } },
  })
}

describe('FeatureGate', () => {
  it('offers nothing to unlock while the edition is not known', () => {
    isLoaded.value = false

    expect(mountGate().text()).toBe('')
  })

  it('shows the locked placeholder once the edition is known to lack the feature', () => {
    isLoaded.value = true

    expect(mountGate().text()).toContain('Incidents')
  })
})
