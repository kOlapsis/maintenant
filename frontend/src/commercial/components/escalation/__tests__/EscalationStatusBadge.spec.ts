// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import type { EscalationRun } from '@/commercial/types/escalation'
import EscalationStatusBadge from '@/commercial/components/escalation/EscalationStatusBadge.vue'

function run(status: string): EscalationRun {
  return {
    id: 'r1',
    policy_id: 'p1',
    policy: { id: 'p1', name: 'On-call' },
    alert_id: 'a1',
    status,
    last_executed_level_index: 0,
    started_at: '',
    ended_at: '',
    next_action_at: null,
  }
}

describe('EscalationStatusBadge', () => {
  it('names a run stopped because its policy was switched off', () => {
    const wrapper = mount(EscalationStatusBadge, { props: { runs: [run('stopped_by_policy_disabled')] } })
    expect(wrapper.text()).toBe('Stopped (policy disabled)')
  })
})
