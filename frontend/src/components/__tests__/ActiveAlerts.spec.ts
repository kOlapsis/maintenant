// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
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

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: vi.fn(), off: vi.fn(), connect: vi.fn(), disconnect: vi.fn() },
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
    for (const fn of Object.values(api)) fn.mockReset()
    api.listRunsForAlert.mockResolvedValue({ runs: [] })
    api.getEscalationRun.mockResolvedValue({ deliveries: [] })
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
})
