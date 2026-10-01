// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AlertList from '@/components/AlertList.vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import type { Alert } from '@/services/alertApi'

function alertAt(id: string, firedAt: string): Alert {
  return {
    id,
    source: 'container',
    alert_type: 'health_unhealthy',
    severity: 'warning',
    status: 'active',
    message: 'unhealthy',
    entity_type: 'container',
    entity_id: id,
    entity_name: id,
    fired_at: firedAt,
    created_at: firedAt,
  }
}

describe('AlertList', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('pages from the last alert listed, not only from its second', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ alerts: [], has_more: false }) })
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())
    const store = useAlertsStore()
    store.alerts = [alertAt('a2', '2026-09-30T10:00:00Z'), alertAt('a1', '2026-09-30T10:00:00Z')]
    store.hasMore = true

    const wrapper = mount(AlertList, {
      global: {
        provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } },
        stubs: { AcknowledgeButton: true },
      },
    })
    const loadMore = wrapper.findAll('button').find((b) => b.text() === 'Load more')
    expect(loadMore).toBeDefined()
    await loadMore!.trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const url = new URL(String(fetchMock.mock.calls[0]![0]))
    expect(url.searchParams.get('before')).toBe('2026-09-30T10:00:00Z')
    expect(url.searchParams.get('before_id')).toBe('a1')
  })
})
