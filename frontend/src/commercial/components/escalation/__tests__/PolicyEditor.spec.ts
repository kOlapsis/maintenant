// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, RouterLinkStub } from '@vue/test-utils'
import type { EscalationPolicy } from '@/commercial/types/escalation'
import PolicyEditor from '@/commercial/components/escalation/PolicyEditor.vue'

const updatePolicy = vi.fn()

vi.mock('@/commercial/stores/escalation', () => ({
  useEscalationStore: () => ({ updatePolicy, createPolicy: vi.fn() }),
}))

vi.mock('@/stores/triggers', () => ({
  useTriggersStore: () => ({ triggersForChannel: () => [], fetchTriggers: vi.fn() }),
}))

vi.mock('@/commercial/composables/useEscalationApi', () => ({
  useEscalationApi: () => ({ overlapProbe: vi.fn().mockResolvedValue({ overlapping: [] }) }),
}))

vi.mock('@/services/apiFetch', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/apiFetch')>()),
  apiFetch: vi.fn().mockResolvedValue({
    channels: [{ id: 'c1', name: 'ops', type: 'webhook', enabled: true }],
  }),
}))

const policy: EscalationPolicy = {
  id: 'p1',
  name: 'Database on-call',
  active: true,
  filters: {
    severities: ['critical'],
    scopes: [
      { kind: 'container', ref_id: 'db-1' },
      { kind: 'endpoint', ref_id: 'api-7' },
    ],
  },
  levels: [{ order: 0, delay_seconds: 300, channel_ids: ['c1'] }],
  created_at: '',
  updated_at: '',
}

describe('PolicyEditor', () => {
  beforeEach(() => {
    updatePolicy.mockReset()
    updatePolicy.mockResolvedValue(policy)
  })

  it('shows the scopes of the policy it edits', async () => {
    const wrapper = mount(PolicyEditor, { props: { policy }, global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()

    const scopes = wrapper.find('[data-test="policy-scopes"]')
    expect(scopes.exists()).toBe(true)
    expect(scopes.text()).toContain('container:db-1')
    expect(scopes.text()).toContain('endpoint:api-7')
  })

  it('saves the policy with its scopes', async () => {
    const wrapper = mount(PolicyEditor, { props: { policy }, global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()

    const save = wrapper.findAll('button').find((b) => b.text().includes('Save policy'))
    expect(save).toBeDefined()
    await save!.trigger('click')
    await flushPromises()

    expect(updatePolicy).toHaveBeenCalledOnce()
    const [id, req] = updatePolicy.mock.calls[0]!
    expect(id).toBe('p1')
    expect(req.filters).toEqual({
      severities: ['critical'],
      scopes: [
        { kind: 'container', ref_id: 'db-1' },
        { kind: 'endpoint', ref_id: 'api-7' },
      ],
    })
  })
})
