import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'

const { fetchContainerUpdate, pinVersion, unpinVersion } = vi.hoisted(() => ({
  fetchContainerUpdate: vi.fn(),
  pinVersion: vi.fn(),
  unpinVersion: vi.fn(),
}))

vi.mock('@/services/updateApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/updateApi')>()),
  fetchContainerUpdate,
}))
vi.mock('@/stores/updates', () => ({
  useUpdatesStore: () => ({ pinVersion, unpinVersion, fetchContainerCves: vi.fn() }),
}))
vi.mock('@/stores/alerts', () => ({
  useAlertsStore: () => ({
    activeAlerts: { critical: [], warning: [], info: [{ id: 'a' }] },
    fetchActiveAlerts: vi.fn(),
  }),
}))
vi.mock('@/stores/agents', () => ({ useAgentsStore: () => ({ hasRemoteAgents: false }) }))
vi.mock('@/composables/useEdition', () => ({ useEdition: () => ({ hasFeature: () => false }) }))

const detail = {
  container_id: 'c1',
  container_name: 'valkey',
  agent_id: '00000000-0000-0000-0000-000000000000',
  current_tag: '8.0.1',
  latest_tag: '8.1.3',
  update_type: 'minor',
  risk_score: 10,
  active_cves: [],
  pinned: true,
  source_url: '',
  previous_digest: '',
  update_command: 'docker compose pull valkey',
  rollback_command: '',
}

function mountPanel() {
  return mount(UpdateDetailPanel, {
    props: { containerId: 'c1' },
    global: {
      stubs: { FeatureGate: true, RiskScoreGauge: true, CveList: true, ChangelogViewer: true, AcknowledgeButton: true },
    },
  })
}

describe('UpdateDetailPanel failures', () => {
  beforeEach(() => {
    for (const fn of [fetchContainerUpdate, pinVersion, unpinVersion]) fn.mockReset()
    fetchContainerUpdate.mockResolvedValue(detail)
  })

  it('shows the error when unpinning fails', async () => {
    unpinVersion.mockRejectedValue(new Error('HTTP 500'))
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'Unpin this version')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 500')
  })

  it('shows the error when the detail cannot be loaded', async () => {
    fetchContainerUpdate.mockRejectedValue(new Error('HTTP 503'))
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('HTTP 503')
  })
})
