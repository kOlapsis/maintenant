// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdatesPage from '@/pages/UpdatesPage.vue'
import { useUpdatesStore } from '@/stores/updates'

const api = vi.hoisted(() => ({
  fetchUpdates: vi.fn(),
  fetchUpdateSummary: vi.fn(),
  triggerScan: vi.fn(),
}))

vi.mock('@/services/updateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/updateApi')>()
  return {
    ...actual,
    ...api,
    fetchHostOS: vi.fn(async () => ({ hosts: [], eol_table: null })),
    fetchImages: vi.fn(async () => ({ images: [] })),
  }
})

vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  return { sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: ref(true) } }
})

vi.mock('@/composables/useEdition', async () => {
  const { ref } = await import('vue')
  return { useEdition: () => ({ edition: ref(null), hasFeature: () => false }) }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, hash: '' }),
  useRouter: () => ({ replace: vi.fn() }),
}))

const counts = { critical: 0, recommended: 0, available: 0, up_to_date: 3, pinned: 0 }

function summary(lastScan: string, status = 'completed') {
  return { last_scan: lastScan, next_scan: '', scan_status: status, counts }
}

async function mountPage() {
  const wrapper = mount(UpdatesPage, { global: { stubs: { FeatureHint: true, FeatureGate: true, teleport: true } } })
  await flushPromises()
  return wrapper
}

const UP_TO_DATE = 'All containers are up to date'

describe('UpdatesPage with nothing to list', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    for (const fn of Object.values(api)) fn.mockReset()
    api.fetchUpdates.mockResolvedValue({ updates: [], last_scan: '', next_scan: '' })
    api.fetchUpdateSummary.mockResolvedValue(summary('2026-11-20T10:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('says everything is up to date after a scan found nothing', async () => {
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain(UP_TO_DATE)
  })

  it('says the list could not be loaded', async () => {
    api.fetchUpdates.mockRejectedValue(new Error('HTTP 503'))
    const wrapper = await mountPage()

    expect(wrapper.text()).not.toContain(UP_TO_DATE)
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
  })

  it('still says up to date after a restart, before the next scan', async () => {
    api.fetchUpdateSummary.mockResolvedValue(summary('0001-01-01T00:00:00Z'))
    const wrapper = await mountPage()

    expect(wrapper.text()).toContain(UP_TO_DATE)
  })

  it('says the last scan did not finish', async () => {
    api.fetchUpdateSummary.mockResolvedValue(summary('2026-11-20T10:00:00Z', 'failed'))
    const wrapper = await mountPage()

    expect(wrapper.text()).not.toContain(UP_TO_DATE)
    expect(wrapper.text()).toContain('The last scan did not finish')
  })

  it('claims nothing while the summary is not loaded', async () => {
    api.fetchUpdateSummary.mockReturnValue(new Promise(() => {}))
    const wrapper = await mountPage()

    expect(wrapper.text()).not.toContain(UP_TO_DATE)
    expect(wrapper.text()).not.toContain('No scan has run yet')
  })

  it('does not claim anything before the first scan', async () => {
    api.fetchUpdateSummary.mockResolvedValue(summary('0001-01-01T00:00:00Z', 'idle'))
    const wrapper = await mountPage()

    expect(wrapper.text()).not.toContain(UP_TO_DATE)
    expect(wrapper.text()).toContain('No scan has run yet')
  })

  it('says a scan could not start', async () => {
    api.triggerScan.mockRejectedValue(new Error('HTTP 403'))
    const wrapper = await mountPage()

    await useUpdatesStore().startScan()
    await flushPromises()

    expect(useUpdatesStore().scanning).toBe(false)
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 403')
  })

  it('keeps showing a scan that runs for more than two minutes', async () => {
    vi.useFakeTimers()
    api.triggerScan.mockResolvedValue(undefined)
    api.fetchUpdateSummary.mockResolvedValue(summary('2026-11-20T10:00:00Z', 'running'))
    const store = useUpdatesStore()

    await store.startScan()
    await vi.advanceTimersByTimeAsync(130_000)
    expect(store.scanning).toBe(true)

    api.fetchUpdateSummary.mockResolvedValue(summary('2026-11-26T10:00:00Z'))
    await vi.advanceTimersByTimeAsync(3_000)
    expect(store.scanning).toBe(false)
  })
})
