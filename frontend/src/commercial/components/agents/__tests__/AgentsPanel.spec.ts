// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { ref } from 'vue'
import AgentsPanel from '@/commercial/components/agents/AgentsPanel.vue'
import type { Agent } from '@/services/agentApi'

const agent: Agent = {
  agent_id: 'a1',
  hostname: 'web-1',
  label: '',
  os_arch: 'linux/amd64',
  agent_version: '1.0.0',
  detected_runtime: 'docker',
  status: 'active',
  connection_state: 'connected',
  last_seen_at: null,
  created_at: '2026-09-30T00:00:00Z',
  revoked_at: null,
  revoked_by: null,
  spool: { queued: 42, draining: true, dropped_since_connect: 7, reported_at: '2026-09-30T00:00:00Z' },
  os: {
    id: 'debian',
    version_id: '12',
    pretty_name: 'Debian 12',
    source: 'host_file',
    unavailable_reason: '',
    reported_at: null,
    support: {
      state: 'supported',
      product: 'debian',
      cycle: '12',
      active_until: null,
      security_until: null,
      extended_until: null,
      days_remaining: null,
      table_source: 'embedded',
    },
  },
}

vi.mock('@/composables/useEdition', () => ({
  useEdition: () => ({
    getQuota: () => ref({ used: 1, limit: -1, isUnlimited: true, isAtLimit: false, nearLimit: false }),
  }),
}))

vi.mock('@/stores/agents', () => ({
  useAgentsStore: () => ({
    agents: [agent],
    tokens: [],
    metrics: null,
    loading: false,
    error: null,
    fetchAgents: vi.fn(),
    fetchTokens: vi.fn(),
    fetchMetrics: vi.fn(),
    connectSSE: vi.fn(),
    disconnectSSE: vi.fn(),
    createToken: vi.fn(),
    deleteToken: vi.fn(),
  }),
}))

describe('AgentsPanel spool badges', () => {
  it('speak the language of the rest of the panel', () => {
    const wrapper = shallowMount(AgentsPanel)
    const text = wrapper.text()

    expect(text).toContain('catching up · 42')
    expect(text).toContain('7 lost')
    expect(wrapper.find('[title="42 events waiting to be replayed"]').exists()).toBe(true)
    expect(wrapper.find('[title="Events dropped because the agent\'s spool was full"]').exists()).toBe(true)
    expect(text).not.toMatch(/rattrapage|perdus/)
  })
})
