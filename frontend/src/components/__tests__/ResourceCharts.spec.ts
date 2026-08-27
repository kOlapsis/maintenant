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

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ref, computed } from 'vue'
import ResourceCharts from '@/components/ResourceCharts.vue'
import { getResourceHistory } from '@/services/resourceApi'
import { ApiError } from '@/services/apiFetch'

vi.mock('@/services/resourceApi', () => ({
  getResourceHistory: vi.fn().mockResolvedValue({ points: [] }),
}))

// uPlot touches matchMedia at import time and is unavailable under jsdom.
vi.mock('@/composables/useChart', () => ({
  useChart: () => ({
    ready: ref(false),
    create: vi.fn(),
    destroy: vi.fn(),
    setData: vi.fn(),
  }),
}))

// The catalogue the engine would report to a Community instance: the whole
// product, with the edition that opens each window.
const CATALOG = [
  { window: '1h', seconds: 3600, min_edition: 'community' },
  { window: '6h', seconds: 21600, min_edition: 'community' },
  { window: '24h', seconds: 86400, min_edition: 'personal' },
  { window: '7d', seconds: 604800, min_edition: 'personal' },
  { window: '30d', seconds: 2592000, min_edition: 'personal' },
  { window: '90d', seconds: 7776000, min_edition: 'pro' },
]

const maxSeconds = ref(21600) // Community
const reload = vi.fn()

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => {
    const historyWindows = computed(() => CATALOG)
    const isWindowOpen = (name: string) => {
      const spec = CATALOG.find((w) => w.window === name)
      return !!spec && spec.seconds <= maxSeconds.value
    }
    return {
      historyWindows,
      isWindowOpen,
      requiredEditionForWindow: (name: string) =>
        CATALOG.find((w) => w.window === name)?.min_edition ?? null,
      largestOpenWindow: computed(
        () => CATALOG.filter((w) => w.seconds <= maxSeconds.value).slice(-1)[0]?.window ?? '',
      ),
      reload,
    }
  },
}))

function mountCharts() {
  return mount(ResourceCharts, {
    props: { containerId: 'c1' },
    global: {
      stubs: { RouterLink: { template: '<a><slot /></a>' }, EditionBadge: true },
    },
  })
}

describe('ResourceCharts window selector', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    maxSeconds.value = 21600
    reload.mockClear()
    vi.mocked(getResourceHistory).mockReset()
    vi.mocked(getResourceHistory).mockResolvedValue({ points: [] } as never)
  })

  // FR-017: a closed window is shown, not hidden. Seeing one hour of history is
  // what makes thirty days worth wanting.
  it('renders every window of the catalogue, open or not', () => {
    const wrapper = mountCharts()
    for (const w of CATALOG) {
      expect(wrapper.find(`[data-test="range-${w.window}"]`).exists()).toBe(true)
    }
  })

  it('marks the windows the edition does not open', () => {
    const wrapper = mountCharts()

    expect(wrapper.find('[data-test="range-1h"]').attributes('data-locked')).toBeUndefined()
    expect(wrapper.find('[data-test="range-6h"]').attributes('data-locked')).toBeUndefined()

    for (const closed of ['24h', '7d', '30d', '90d']) {
      const button = wrapper.find(`[data-test="range-${closed}"]`)
      expect(button.attributes('data-locked')).toBe('true')
      expect(button.attributes('disabled')).toBeDefined()
    }
  })

  // FR-018: the interface never fires a request it knows will be refused.
  it('does not fetch when a closed window is clicked', async () => {
    const wrapper = mountCharts()
    await wrapper.vm.$nextTick()
    vi.mocked(getResourceHistory).mockClear()

    await wrapper.find('[data-test="range-30d"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(getResourceHistory).not.toHaveBeenCalled()
  })

  it('fetches when an open window is clicked', async () => {
    const wrapper = mountCharts()
    await wrapper.vm.$nextTick()
    vi.mocked(getResourceHistory).mockClear()

    await wrapper.find('[data-test="range-6h"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(getResourceHistory).toHaveBeenCalledWith('c1', '6h')
  })

  // The cap comes from the server, so a Personal instance opens more without the
  // component knowing anything about editions.
  it('opens the windows a higher edition unlocks', () => {
    maxSeconds.value = 2592000 // Personal
    const wrapper = mountCharts()

    expect(wrapper.find('[data-test="range-30d"]').attributes('data-locked')).toBeUndefined()
    expect(wrapper.find('[data-test="range-90d"]').attributes('data-locked')).toBe('true')
  })
})

// A license can expire between two refreshes. The refusal is the signal: the
// view falls back rather than breaking (FR-021b, US5).
describe('ResourceCharts when the window closes under the view', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    reload.mockClear()
    vi.mocked(getResourceHistory).mockReset()
  })

  it('falls back to the largest open window and says so', async () => {
    // Pro at mount: 90d is selectable.
    maxSeconds.value = 7776000
    vi.mocked(getResourceHistory).mockResolvedValue({ points: [] } as never)

    const wrapper = mountCharts()
    await wrapper.vm.$nextTick()
    await wrapper.find('[data-test="range-90d"]').trigger('click')
    await wrapper.vm.$nextTick()

    // The license drops to Community, and the next refresh is refused.
    vi.mocked(getResourceHistory).mockRejectedValue(
      new ApiError(403, { code: 'EDITION_REQUIRED', message: 'nope', required_edition: 'pro' }, 'nope'),
    )
    reload.mockImplementation(async () => {
      maxSeconds.value = 21600
    })

    await wrapper.find('[data-test="range-30d"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()

    expect(reload).toHaveBeenCalled()
    const notice = wrapper.find('[data-test="window-closed-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('6h')
  })

  it('does not show the notice for an ordinary failure', async () => {
    vi.mocked(getResourceHistory).mockRejectedValue(new Error('network down'))

    const wrapper = mountCharts()
    await new Promise((r) => setTimeout(r, 0))
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="window-closed-notice"]').exists()).toBe(false)
    expect(reload).not.toHaveBeenCalled()
  })
})
