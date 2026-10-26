// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusComponentBreakdown from '@/components/StatusComponentBreakdown.vue'
import type { MonitorRef } from '@/services/statusApi'

// 043-public-surfaces-hardening, US1 scenario 4, FR-004 and the blank-label edge case.

describe('StatusComponentBreakdown', () => {
  it('names a monitor by its public label, else by its type, never by an identifier (FR-004)', () => {
    const monitors = [
      { type: 'endpoint', status: 'operational', label: 'API publique' },
      { type: 'heartbeat', status: 'major_outage' },
      { type: 'certificate', status: 'operational', label: '   ' },
      { type: 'container', status: 'degraded', label: 'Base de données' },
    ] as unknown as MonitorRef[]

    const text = mount(StatusComponentBreakdown, { props: { monitors } }).text()

    expect(text).toContain('API publique')
    expect(text).toContain('Base de données')
    expect(text).toContain('Heartbeat')
    expect(text).toContain('Certificate')
    expect(text).not.toContain('#')
    expect(text).not.toContain('undefined')
  })

  it('never shows an identifier or a target, even when the payload carries one', () => {
    const monitors = [
      { type: 'heartbeat', id: '4f1ebbbb-0000-4000-8000-000000000002', status: 'operational' },
      { type: 'endpoint', id: '0195aaaa-0000-7000-8000-000000000001', name: 'http://api.internal:3000/health', status: 'operational' },
    ]

    const text = mount(StatusComponentBreakdown, { props: { monitors } }).text()

    expect(text).not.toContain('4f1ebbbb')
    expect(text).not.toContain('0195aaaa')
    expect(text).not.toContain('api.internal')
    expect(text).not.toContain('#')
    expect(text).toContain('Heartbeat')
    expect(text).toContain('Endpoint')
  })
})
