// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ActiveAlerts from '@/components/ActiveAlerts.vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import type { Alert } from '@/services/alertApi'

const api = vi.hoisted(() => ({
  getActiveAlerts: vi.fn(),
  listRunsForAlert: vi.fn(),
  getEscalationRun: vi.fn(),
}))

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return { ...actual, getActiveAlerts: api.getActiveAlerts }
})

vi.mock('@/commercial/composables/useEscalationApi', () => ({
  useEscalationApi: () => ({ listRunsForAlert: api.listRunsForAlert, getEscalationRun: api.getEscalationRun }),
}))

const edition = vi.hoisted(() => ({ escalation: { value: true } }))
vi.mock('@/composables/useEdition', async () => {
  const { ref } = await import('vue')
  edition.escalation = ref(true)
  return { useEdition: () => ({ hasFeature: () => edition.escalation.value }) }
})

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn(), connected: { value: true } },
}))

function alert(id: string, over: Partial<Alert> = {}): Alert {
  return {
    id,
    source: 'container',
    alert_type: 'down',
    severity: 'critical',
    status: 'active',
    message: `${id} is down`,
    entity_type: 'container',
    entity_id: `e-${id}`,
    entity_name: id,
    fired_at: '2026-11-26T10:00:00Z',
    created_at: '2026-11-26T10:00:00Z',
    ...over,
  } as Alert
}

function run(alertId: string, over: Record<string, unknown> = {}) {
  return {
    id: `run-${alertId}`,
    policy_id: 'p1',
    policy: { id: 'p1', name: 'On call' },
    alert_id: alertId,
    status: 'active',
    last_executed_level_index: 0,
    started_at: '2026-11-26T10:00:00Z',
    ended_at: null,
    next_action_at: null,
    ...over,
  }
}

function mountList() {
  return mount(ActiveAlerts, {
    global: { provide: { [detailSlideOverKey as symbol]: { openDetail: vi.fn() } } },
  })
}

describe('ActiveAlerts', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    edition.escalation.value = true
    for (const fn of Object.values(api)) fn.mockReset()
    api.listRunsForAlert.mockResolvedValue({ runs: [] })
    api.getEscalationRun.mockResolvedValue({ deliveries: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('says the active alerts could not be loaded', async () => {
    api.getActiveAlerts.mockRejectedValue(new Error('HTTP 503'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await useAlertsStore().fetchActiveAlerts()

    const wrapper = mountList()

    expect(wrapper.text()).not.toContain('No active alerts')
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
  })

  it('says there is no active alert when the list loaded empty', async () => {
    api.getActiveAlerts.mockResolvedValue({ critical: [], warning: [], info: [] })
    await useAlertsStore().fetchActiveAlerts()

    expect(mountList().text()).toContain('No active alerts')
  })

  it('shows the escalation of an alert that arrives after the page', async () => {
    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1')] })
    api.getEscalationRun.mockResolvedValue(run('a1'))
    const wrapper = mountList()
    await flushPromises()
    expect(api.listRunsForAlert).not.toHaveBeenCalled()

    useAlertsStore().activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    await flushPromises()

    expect(wrapper.text()).toContain('On call')
    expect(wrapper.text()).toContain('Level 1')
  })

  it('reloads the escalation once the alert is acknowledged', async () => {
    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1')] })
    api.getEscalationRun.mockResolvedValue(run('a1'))
    const store = useAlertsStore()
    store.activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await flushPromises()

    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1', { status: 'stopped_by_ack' })] })
    api.getEscalationRun.mockResolvedValue(run('a1', { status: 'stopped_by_ack' }))
    store.activeAlerts = { critical: [alert('a1', { acknowledged_at: '2026-11-26T10:05:00Z' })], warning: [], info: [] }
    await flushPromises()

    expect(api.listRunsForAlert).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Stopped (ack)')
  })

  it('says when a notification of the escalation was not delivered', async () => {
    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1')] })
    api.getEscalationRun.mockResolvedValue(
      run('a1', { deliveries: [{ id: 'd1', status: 'sent' }, { id: 'd2', status: 'failed' }, { id: 'd3', status: 'abandoned' }] }),
    )
    useAlertsStore().activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await flushPromises()

    expect(wrapper.text()).toContain('2 not delivered')
  })

  it('keeps the load error above a live alert and offers to retry', async () => {
    api.getActiveAlerts.mockRejectedValueOnce(new Error('HTTP 502'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useAlertsStore()
    await store.fetchActiveAlerts()
    store.activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 502')
    expect(wrapper.text()).toContain('a1 is down')

    api.getActiveAlerts.mockResolvedValue({ critical: [alert('a1'), alert('a2')], warning: [], info: [] })
    await wrapper.findAll('button').find((b) => b.text() === 'Retry')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('a2 is down')
  })

  it('keeps the escalation when its detail fails, and asks for the detail again later', async () => {
    vi.useFakeTimers()
    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1')] })
    api.getEscalationRun
      .mockRejectedValueOnce(new Error('HTTP 429'))
      .mockResolvedValue(run('a1', { deliveries: [{ id: 'd1', status: 'failed' }] }))
    useAlertsStore().activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await vi.advanceTimersByTimeAsync(0)

    expect(wrapper.text()).toContain('On call')
    expect(wrapper.text()).not.toContain('not delivered')

    await vi.advanceTimersByTimeAsync(30_000)

    expect(wrapper.text()).toContain('1 not delivered')
    wrapper.unmount()
  })

  it('stops asking once the page is left', async () => {
    vi.useFakeTimers()
    api.listRunsForAlert.mockRejectedValue(new Error('HTTP 503'))
    useAlertsStore().activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await vi.advanceTimersByTimeAsync(0)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(600_000)

    expect(api.listRunsForAlert).toHaveBeenCalledTimes(1)
  })

  it('asks for no escalation until the edition offers it', async () => {
    edition.escalation.value = false
    api.listRunsForAlert.mockResolvedValue({ runs: [run('a1')] })
    api.getEscalationRun.mockResolvedValue(run('a1'))
    useAlertsStore().activeAlerts = { critical: [alert('a1')], warning: [], info: [] }
    const wrapper = mountList()
    await flushPromises()
    expect(api.listRunsForAlert).not.toHaveBeenCalled()

    edition.escalation.value = true
    await flushPromises()

    expect(wrapper.text()).toContain('On call')
  })
})
