// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AlertList from '@/components/AlertList.vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import type { Alert } from '@/services/alertApi'
import { LOCAL_AGENT } from '@/services/apiFetch'

const sse = vi.hoisted(() => new Map<string, (e: MessageEvent) => void>())
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => sse.set(type, fn),
    off: (type: string) => sse.delete(type),
    connect: () => {},
    disconnect: () => {},
    connected: false,
  },
}))

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
    agent_id: LOCAL_AGENT,
    fired_at: firedAt,
    created_at: firedAt,
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
}

function mountList(props: { linkedId?: string } = {}) {
  return mount(AlertList, {
    props,
    attachTo: document.body,
    global: {
      provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } },
      stubs: { AcknowledgeButton: true },
    },
  })
}

describe('AlertList', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('highlights the linked alert when it is in the loaded page', async () => {
    const fetchMock = vi.fn().mockImplementation(async () => jsonResponse(alertAt('a1', '2026-09-30T10:00:00Z')))
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())
    const store = useAlertsStore()
    store.alerts = [alertAt('a2', '2026-09-30T11:00:00Z'), alertAt('a1', '2026-09-30T10:00:00Z')]

    const wrapper = mountList({ linkedId: 'a1' })
    await flushPromises()

    const rows = wrapper.findAll('tr[data-alert-id]')
    expect(rows.map((r) => r.attributes('data-alert-id'))).toEqual(['a2', 'a1'])
    expect(rows[1]!.classes()).toContain('alert-linked')
    expect(rows[0]!.classes()).not.toContain('alert-linked')
  })

  it('fetches the linked alert by id and shows it above a page that does not hold it', async () => {
    const fetchMock = vi.fn().mockImplementation(async () => jsonResponse(alertAt('old', '2026-08-01T10:00:00Z')))
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())
    const store = useAlertsStore()
    store.alerts = [alertAt('a2', '2026-09-30T11:00:00Z')]

    const wrapper = mountList({ linkedId: 'old' })
    await flushPromises()

    expect(String(fetchMock.mock.calls[0]![0])).toMatch(/\/alerts\/old$/)
    const rows = wrapper.findAll('tr[data-alert-id]')
    expect(rows.map((r) => r.attributes('data-alert-id'))).toEqual(['old', 'a2'])
    expect(rows[0]!.classes()).toContain('alert-linked')
  })

  it('applies live updates to a linked alert fetched outside the loaded page', async () => {
    const old = alertAt('old', '2026-08-01T10:00:00Z')
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => jsonResponse(old)))
    setActivePinia(createPinia())
    const store = useAlertsStore()
    store.alerts = [alertAt('a2', '2026-09-30T11:00:00Z')]
    store.connectSSE()

    const wrapper = mountList({ linkedId: 'old' })
    await flushPromises()

    sse.get('alert.acknowledged')!({ data: JSON.stringify({ ...old, acknowledged_at: '2026-10-01T09:00:00Z', acknowledged_by: 'ops' }) } as MessageEvent)
    expect(store.alerts.find((a) => a.id === 'old')!.acknowledged_at).toBe('2026-10-01T09:00:00Z')

    sse.get('alert.resolved')!({ data: JSON.stringify({ ...old, status: 'resolved', resolved_at: '2026-10-01T09:05:00Z' }) } as MessageEvent)
    await flushPromises()
    const row = wrapper.find('tr[data-alert-id="old"]')
    expect(row.classes()).toContain('alert-linked')
    expect(row.text()).toContain('resolved')
    store.disconnectSSE()
  })

  it('updates a linked alert outside the loaded page when it is acknowledged from its row', async () => {
    const old = alertAt('old', '2026-08-01T10:00:00Z')
    const acked = { ...old, acknowledged_at: '2026-10-01T09:00:00Z', acknowledged_by: 'maintenant-ui' }
    const fetchMock = vi.fn().mockImplementation(async (url: string) => jsonResponse(String(url).endsWith('/acknowledge') ? acked : old))
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())
    const store = useAlertsStore()
    store.alerts = [alertAt('a2', '2026-09-30T11:00:00Z')]

    mountList({ linkedId: 'old' })
    await flushPromises()
    await store.acknowledgeAlert('old')

    expect(store.alerts.find((a) => a.id === 'old')!.acknowledged_at).toBe('2026-10-01T09:00:00Z')
  })

  it('keeps the linked alert when the first page arrives after it, and drops it on a filter change', async () => {
    const old = alertAt('old', '2026-08-01T10:00:00Z')
    const page = { alerts: [alertAt('a2', '2026-09-30T11:00:00Z')], has_more: false }
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string) => jsonResponse(String(url).includes('/alerts/old') ? old : page)))
    setActivePinia(createPinia())
    const store = useAlertsStore()

    const wrapper = mountList({ linkedId: 'old' })
    await flushPromises()
    await store.fetchAlerts()
    expect(store.alerts.map((a) => a.id)).toEqual(['old', 'a2'])

    await wrapper.findAllComponents({ name: 'SelectInput' })[0]!.vm.$emit('update:modelValue', 'endpoint')
    await flushPromises()
    expect(store.alerts.map((a) => a.id)).toEqual(['a2'])
  })

  it('says so when the linked alert no longer exists', async () => {
    const fetchMock = vi.fn().mockImplementation(async () =>
      jsonResponse({ error: { code: 'NOT_FOUND', message: 'alert not found' } }, 404),
    )
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())

    const wrapper = mountList({ linkedId: 'gone' })
    await flushPromises()

    expect(wrapper.text()).toContain('Linked alert not found')
    expect(wrapper.text()).toContain('This alert no longer exists')
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
