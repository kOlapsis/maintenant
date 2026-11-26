// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UpdatesPage from '@/pages/UpdatesPage.vue'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'
import AgentBadge from '@/components/AgentBadge.vue'
import { useAgentsStore } from '@/stores/agents'
import { useResourcesStore } from '@/stores/resources'
import { useAlertsStore } from '@/stores/alerts'
import type { Agent } from '@/services/agentApi'
import { acknowledgeAlert, type Alert } from '@/services/alertApi'
import type { ImageUpdate } from '@/services/updateApi'

const api = vi.hoisted(() => ({
  updates: [] as unknown[],
  agents: [] as unknown[],
  details: {} as Record<string, unknown>,
  alerts: [] as Array<{ id: string }>,
}))

vi.mock('@/services/updateApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/updateApi')>()
  return {
    ...actual,
    fetchUpdates: vi.fn(async () => ({ updates: api.updates, last_scan: '', next_scan: '' })),
    fetchUpdateSummary: vi.fn(async () => ({
      last_scan: '',
      next_scan: '',
      scan_status: 'idle',
      counts: {
        critical: 0,
        recommended: api.updates.length,
        available: 0,
        up_to_date: 0,
        pinned: 0,
      },
    })),
    fetchHostOS: vi.fn(async () => ({ hosts: [], eol_table: null })),
    fetchContainerUpdate: vi.fn(async (id: string) => api.details[id]),
    fetchContainerCves: vi.fn(async (id: string) => ({ container_id: id, cves: [] })),
  }
})

vi.mock('@/services/agentApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/agentApi')>()
  return { ...actual, listAgents: vi.fn(async () => ({ agents: api.agents })) }
})

vi.mock('@/services/alertApi', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/alertApi')>()
  return {
    ...actual,
    getActiveAlerts: vi.fn(async () => ({ critical: [], warning: [], info: api.alerts })),
    acknowledgeAlert: vi.fn(async (id: string, by: string) => ({
      ...api.alerts.find((a) => a.id === id),
      acknowledged_at: '2026-10-13T11:00:00Z',
      acknowledged_by: by,
    })),
  }
})

vi.mock('@/services/sseBus', async () => {
  const { ref } = await import('vue')
  return {
    sseBus: {
      on: vi.fn(),
      off: vi.fn(),
      connect: vi.fn(),
      disconnect: vi.fn(),
      connected: ref(true),
    },
  }
})

vi.mock('@/composables/useEdition', async () => {
  const { ref } = await import('vue')
  return { useEdition: () => ({ edition: ref(null), hasFeature: () => false }) }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, hash: '' }),
  useRouter: () => ({ replace: vi.fn() }),
}))

type Host = Pick<ImageUpdate, 'agent_id' | 'agent_hostname' | 'agent_label'>

function entry(name: string, containerId: string, host: Host): ImageUpdate {
  return {
    id: `upd-${containerId}`,
    container_id: containerId,
    container_name: name,
    image: `${name}/${name}:8.0.1`,
    current_tag: '8.0.1',
    current_digest: '',
    latest_tag: '8.1.3',
    latest_digest: '',
    update_type: 'minor',
    published_at: null,
    changelog_url: '',
    changelog_summary: '',
    has_breaking_changes: false,
    risk_score: 50,
    status: 'available',
    detected_at: '2026-10-13T10:00:00Z',
    ...host,
  }
}

function detail(e: ImageUpdate) {
  return {
    ...e,
    active_cves: [],
    pinned: false,
    source_url: '',
    previous_digest: '',
    update_command: `docker compose pull ${e.container_name}`,
    rollback_command: '',
  }
}

function enrolled(agentId: string, hostname: string, label: string): Agent {
  return { agent_id: agentId, hostname, label, status: 'active' } as Agent
}

const LOCAL_AGENT_ID = '00000000-0000-0000-0000-000000000000'

function activeUpdateAlert(id: string, agentId: string): Alert {
  return {
    id,
    source: 'update',
    alert_type: 'update_available',
    severity: 'info',
    status: 'active',
    message: 'Update available for valkey: 8.1.3',
    entity_type: 'container',
    entity_id: `uid-${id}`,
    entity_name: 'valkey',
    agent_id: agentId,
    fired_at: '2026-10-13T10:00:00Z',
    created_at: '2026-10-13T10:00:00Z',
  }
}

const nas: Host = { agent_id: 'agent-nas', agent_hostname: 'nas-02', agent_label: 'NAS cave' }
const web: Host = { agent_id: 'agent-web', agent_hostname: 'web-07' }

async function mountPage(updates: ImageUpdate[], agents: Agent[]) {
  api.updates = updates
  api.agents = agents
  api.details = Object.fromEntries(updates.map((u) => [u.container_id, detail(u)]))
  useAgentsStore().agents = agents
  const wrapper = mount(UpdatesPage, {
    global: {
      stubs: {
        teleport: true,
        FeatureGate: true,
        FeatureHint: true,
        OsSupportTimeline: true,
        RiskScoreGauge: true,
        CveList: true,
        ChangelogViewer: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

function hostBadges(w: VueWrapper | ReturnType<VueWrapper['findComponent']>): string[] {
  return w
    .findAllComponents(AgentBadge)
    .map((b) => b.text().trim())
    .filter((t) => t !== '')
    .sort()
}

async function mountPanel(e: ImageUpdate, alerts: Alert[]) {
  api.alerts = alerts
  await useAlertsStore().fetchActiveAlerts()
  api.details = { [e.container_id]: detail(e) }
  useAgentsStore().agents = [
    enrolled('agent-nas', 'nas-02', 'NAS cave'),
    enrolled('agent-web', 'web-07', ''),
  ]
  const wrapper = mount(UpdateDetailPanel, {
    props: { containerId: e.container_id },
    global: {
      stubs: { FeatureGate: true, RiskScoreGauge: true, CveList: true, ChangelogViewer: true },
    },
  })
  await flushPromises()
  expect(wrapper.text()).toContain(`docker compose pull ${e.container_name}`)
  return wrapper
}

function ackButtons(w: VueWrapper) {
  return w.findAll('button').filter((b) => b.text() === 'Acknowledge')
}

async function openEntry(wrapper: VueWrapper, containerName: string) {
  const label = wrapper.findAll('p').find((p) => p.text() === containerName)
  expect(label, `entry ${containerName} is listed`).toBeDefined()
  await label!.trigger('click')
  await flushPromises()
  const panel = wrapper.findComponent(UpdateDetailPanel)
  expect(panel.exists()).toBe(true)
  expect(panel.text()).toContain(`docker compose pull ${containerName}`)
  return panel
}

describe('UpdatesPage: host of each update (042)', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    vi.clearAllMocks()
    api.alerts = []
  })

  // FR-002, US1 scenario 1, edge cases "Conteneur local" and "Agent inconnu".
  it('names the host of every entry on a multi-host install', async () => {
    const wrapper = await mountPage(
      [
        entry('valkey', 'ctr-valkey-local', {}),
        entry('valkey', 'ctr-valkey-nas', nas),
        entry('valkey', 'ctr-valkey-web', web),
        entry('valkey', 'ctr-valkey-ghost', { agent_id: 'agent-ghost' }),
      ],
      [enrolled('agent-nas', 'nas-02', 'NAS cave'), enrolled('agent-web', 'web-07', '')],
    )

    expect(wrapper.findAll('p').filter((p) => p.text() === 'valkey')).toHaveLength(4)
    expect(hostBadges(wrapper)).toEqual(['Local', 'NAS cave', 'agent-ghost', 'web-07'].sort())
  })

  // Contract "Interface": the badge stays whatever the global host filter.
  it('keeps the host on every entry when a global host filter is set', async () => {
    useResourcesStore().setFilter('agent-nas')
    const wrapper = await mountPage(
      [entry('redis', 'ctr-redis-web', web), entry('redis', 'ctr-redis-local', {})],
      [enrolled('agent-nas', 'nas-02', 'NAS cave'), enrolled('agent-web', 'web-07', '')],
    )

    expect(hostBadges(wrapper)).toEqual(['Local', 'web-07'])
  })

  // US1 scenario 4, FR-007, SC-004.
  it('shows no host on an install without remote agent', async () => {
    const wrapper = await mountPage(
      [entry('valkey', 'ctr-valkey-local', {}), entry('pg-main', 'ctr-pg-local', {})],
      [],
    )

    expect(
      wrapper.findAll('p').filter((p) => ['valkey', 'pg-main'].includes(p.text())),
    ).toHaveLength(2)
    expect(hostBadges(wrapper)).toEqual([])
    expect(wrapper.text()).not.toContain('Local')
  })

  // US1 scenario 2, FR-002: the detail panel names the host of the opened entry.
  it.each([
    ['valkey', 'NAS cave'],
    ['pg-main', 'Local'],
  ])('names the host in the detail panel of %s', async (containerName, host) => {
    const wrapper = await mountPage(
      [entry('valkey', 'ctr-valkey-nas', nas), entry('pg-main', 'ctr-pg-local', {})],
      [enrolled('agent-nas', 'nas-02', 'NAS cave')],
    )

    const panel = await openEntry(wrapper, containerName)
    expect(hostBadges(panel)).toEqual([host])
  })

  // US1 scenario 4 on the detail panel.
  it('shows no host in the detail panel without remote agent', async () => {
    const wrapper = await mountPage([entry('pg-main', 'ctr-pg-local', {})], [])

    const panel = await openEntry(wrapper, 'pg-main')
    expect(hostBadges(panel)).toEqual([])
    expect(panel.text()).not.toContain('Local')
  })

  // US2 scenario 6, FR-010, contract "Interface": with one valkey alert per host,
  // the panel acknowledges the alert of the opened entry's host, the local runtime
  // included (no agent field on the entry, sentinel on the alert). The target
  // alert is listed last so that a match on the name alone picks another one.
  it.each([
    ['NAS cave', entry('valkey', 'ctr-valkey-nas', nas), 'alert-nas'],
    ['web-07', entry('valkey', 'ctr-valkey-web', web), 'alert-web'],
    ['Local', entry('valkey', 'ctr-valkey-local', {}), 'alert-local'],
  ])(
    'acknowledges the update alert of its own host from the %s entry',
    async (_host, e, alertId) => {
      const all = [
        activeUpdateAlert('alert-local', LOCAL_AGENT_ID),
        activeUpdateAlert('alert-nas', 'agent-nas'),
        activeUpdateAlert('alert-web', 'agent-web'),
      ]
      const panel = await mountPanel(e, [
        ...all.filter((a) => a.id !== alertId),
        ...all.filter((a) => a.id === alertId),
      ])

      const buttons = ackButtons(panel)
      expect(buttons).toHaveLength(1)
      await buttons[0]!.trigger('click')
      await flushPromises()

      expect(vi.mocked(acknowledgeAlert).mock.calls.map((c) => c[0])).toEqual([alertId])
      const acked = useAlertsStore()
        .activeAlerts.info.filter((a) => a.acknowledged_at)
        .map((a) => a.id)
      expect(acked).toEqual([alertId])
    },
  )

  // FR-010: the panel never offers to acknowledge another host's alert.
  it.each([
    [
      'web-07',
      entry('valkey', 'ctr-valkey-web', web),
      [activeUpdateAlert('alert-nas', 'agent-nas')],
    ],
    [
      'Local',
      entry('valkey', 'ctr-valkey-local', {}),
      [activeUpdateAlert('alert-web', 'agent-web')],
    ],
  ])(
    'offers no acknowledgement on the %s entry when only another host has an alert',
    async (_host, e, alerts) => {
      const panel = await mountPanel(e, alerts)

      expect(ackButtons(panel)).toHaveLength(0)
      expect(panel.text()).not.toContain('Acknowledged')
      expect(vi.mocked(acknowledgeAlert)).not.toHaveBeenCalled()
    },
  )
})
