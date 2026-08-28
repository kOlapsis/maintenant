// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref, computed } from 'vue'
import TopConsumersWidget from '@/components/TopConsumersWidget.vue'

const CATALOG = [
  { window: '1h', seconds: 3600, min_edition: 'community' },
  { window: '6h', seconds: 21600, min_edition: 'community' },
  { window: '24h', seconds: 86400, min_edition: 'personal' },
  { window: '7d', seconds: 604800, min_edition: 'personal' },
  { window: '30d', seconds: 2592000, min_edition: 'personal' },
  { window: '90d', seconds: 7776000, min_edition: 'pro' },
]

const maxSeconds = ref(21600) // Community

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    historyWindows: computed(() => CATALOG),
    isWindowOpen: (name: string) => {
      const spec = CATALOG.find((w) => w.window === name)
      return !!spec && spec.seconds <= maxSeconds.value
    },
    requiredEditionForWindow: (name: string) =>
      CATALOG.find((w) => w.window === name)?.min_edition ?? null,
  }),
}))

function mountWidget() {
  return mount(TopConsumersWidget, {
    props: { metric: 'cpu' as const, period: '1h', consumers: [] },
  })
}

describe('TopConsumersWidget periods', () => {
  beforeEach(() => {
    maxSeconds.value = 21600
  })

  // The widget used to hold its own free/paid split, which is what let a
  // direct call reach a paid period the server never checked.
  it('builds its periods from the catalogue, not from a hardcoded list', () => {
    const wrapper = mountWidget()
    for (const w of CATALOG) {
      expect(wrapper.find(`[data-test="period-${w.window}"]`).exists()).toBe(true)
    }
  })

  it('locks the periods the edition does not open', () => {
    const wrapper = mountWidget()

    expect(wrapper.find('[data-test="period-6h"]').attributes('data-locked')).toBeUndefined()
    for (const closed of ['24h', '7d', '30d', '90d']) {
      const button = wrapper.find(`[data-test="period-${closed}"]`)
      expect(button.attributes('data-locked')).toBe('true')
      expect(button.attributes('disabled')).toBeDefined()
    }
  })

  it('does not emit for a locked period', async () => {
    const wrapper = mountWidget()
    await wrapper.find('[data-test="period-30d"]').trigger('click')
    expect(wrapper.emitted('update:period')).toBeUndefined()
  })

  it('emits for an open period', async () => {
    const wrapper = mountWidget()
    await wrapper.find('[data-test="period-6h"]').trigger('click')
    expect(wrapper.emitted('update:period')).toEqual([['6h']])
  })

  it('follows the cap the server reports', async () => {
    maxSeconds.value = 7776000 // Pro
    const wrapper = mountWidget()

    expect(wrapper.find('[data-test="period-90d"]').attributes('data-locked')).toBeUndefined()
    await wrapper.find('[data-test="period-90d"]').trigger('click')
    expect(wrapper.emitted('update:period')).toEqual([['90d']])
  })
})
