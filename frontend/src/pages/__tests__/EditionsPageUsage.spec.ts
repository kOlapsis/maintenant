// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import EditionsPage from '@/pages/EditionsPage.vue'
import { useEdition } from '@/composables/useEdition'
import { editionPayload } from '@/components/__tests__/communityLimitFixtures'

// 051-community-label-limits, US2 scenario 8, FR-017: the Editions page counts
// the endpoints and certificates of every origin against the cap, and says how
// many are not monitored.

vi.mock('@/services/editionApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/editionApi')>()),
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))
vi.mock('@/composables/useFeedbackUrl', async () => {
  const { computed } = await import('vue')
  return { useFeedbackUrl: () => ({ feedbackUrl: computed(() => '') }) }
})

function mountPage() {
  return mount(EditionsPage, { global: { stubs: { RouterLink: RouterLinkStub } } })
}

describe('EditionsPage usage', () => {
  for (const tc of [
    { used: 17, unmonitored: 6, certUsed: 8, certUnmonitored: 4 },
    { used: 23, unmonitored: 13, certUsed: 7, certUnmonitored: 2 },
  ]) {
    it(`shows ${tc.used} endpoints counted against 10, ${tc.unmonitored} not monitored`, () => {
      useEdition().edition.value = editionPayload('community', {
        endpoints: { used: tc.used, limit: 10, unmonitored: tc.unmonitored },
        certificates: { used: tc.certUsed, limit: 5, unmonitored: tc.certUnmonitored },
      })
      const text = mountPage().text()

      expect(text).toMatch(new RegExp(`\\b${tc.used}\\b`))
      expect(text).toMatch(new RegExp(`\\b${tc.unmonitored}\\b`))
      expect(text).toMatch(new RegExp(`\\b${tc.certUsed}\\b`))
      expect(text).toMatch(new RegExp(`\\b${tc.certUnmonitored}\\b`))
      expect(text).toMatch(/not monitored/i)
    })
  }
})
