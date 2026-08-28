// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { buildAttentionItems } from '../useAttentionItems'
import { UPDATES_ROLLUP_ID } from '../attentionAggregator'
import type { UnifiedMonitor } from '@/stores/dashboard'
import type { Alert } from '@/services/alertApi'

const NOW = Date.parse('2026-06-13T12:00:00Z')

function monitor(p: Partial<UnifiedMonitor> & Pick<UnifiedMonitor, 'id' | 'type' | 'name' | 'status'>): UnifiedMonitor {
  return {
    statusLabel: 'X',
    subtitle: '',
    group: null,
    sparklineData: null,
    sparklineType: null,
    metricValue: null,
    metricLabel: null,
    link: { name: 'containers' },
    updatedAt: '2026-06-13T11:30:00Z',
    ...p,
  } as UnifiedMonitor
}

function alert(p: Partial<Alert> & Pick<Alert, 'severity' | 'entity_type' | 'entity_id'>): Alert {
  return {
    id: `alert-${p.entity_id || p.source || 's'}`,
    source: 'security',
    alert_type: 'dangerous_configuration',
    status: 'active',
    message: 'Security issue detected',
    entity_name: p.entity_id,
    fired_at: '2026-06-13T11:00:00Z',
    created_at: '2026-06-13T11:00:00Z',
    ...p,
  } as Alert
}

const updateAlert = (id: string): Alert =>
  alert({ id, source: 'update', alert_type: 'update_available', severity: 'critical', entity_type: 'container', entity_id: '', entity_name: 'svc' })

describe('buildAttentionItems', () => {
  it('keeps only unhealthy monitors (incident/warning), drops ok/paused/unknown', () => {
    const items = buildAttentionItems(
      [
        monitor({ id: 'container:a', type: 'container', name: 'a', status: 'down' }),
        monitor({ id: 'endpoint:b', type: 'endpoint', name: 'b', status: 'warning' }),
        monitor({ id: 'container:c', type: 'container', name: 'c', status: 'ok' }),
        monitor({ id: 'heartbeat:d', type: 'heartbeat', name: 'd', status: 'paused' }),
        monitor({ id: 'certificate:e', type: 'certificate', name: 'e', status: 'unknown' }),
      ],
      [],
      0,
      NOW,
    )
    expect(items.map((i) => i.name)).toEqual(['a', 'b'])
    expect(items[0]!.severity).toBe('incident')
    expect(items[1]!.severity).toBe('warning')
  })

  it('routes slide-over types vs plain routes', () => {
    const items = buildAttentionItems(
      [
        monitor({ id: 'container:a', type: 'container', name: 'a', status: 'down' }),
        monitor({ id: 'workload:w', type: 'workload', name: 'w', status: 'down' }),
      ],
      [],
      0,
      NOW,
    )
    expect(items.find((i) => i.name === 'a')!.nav.slideOver).toEqual({ type: 'container', id: 'a' })
    expect(items.find((i) => i.name === 'w')!.nav.route).toBeDefined()
  })

  it('mirrors the alert severity (no invention): critical → incident, warning → warning, info dropped', () => {
    const items = buildAttentionItems(
      [],
      [
        alert({ severity: 'critical', entity_type: 'container', entity_id: 'sec', entity_name: 'sec' }),
        alert({ source: 'agent', alert_type: 'disconnected', severity: 'warning', entity_type: 'agent', entity_id: 'a1', entity_name: 'host-1' }),
        alert({ severity: 'info', entity_type: 'container', entity_id: 'i', entity_name: 'i' }),
      ],
      0,
      NOW,
    )
    const sec = items.find((i) => i.id === 'container:sec')!
    expect(sec.severity).toBe('incident')
    expect(sec.kind).toBe('Security')
    expect(sec.nav.slideOver).toEqual({ type: 'container', id: 'sec' })
    const ag = items.find((i) => i.id === 'agent:a1')!
    expect(ag.severity).toBe('warning')
    expect(ag.kind).toBe('Agent')
    expect(ag.nav.route).toEqual({ name: 'agents' })
    expect(items.some((i) => i.id === 'container:i')).toBe(false)
  })

  it('rolls up critical update alerts into one incident entry, relabelled with the scanner count', () => {
    const items = buildAttentionItems([], [updateAlert('u1'), updateAlert('u2')], 7, NOW)
    expect(items).toHaveLength(1)
    const rollup = items[0]!
    expect(rollup.id).toBe(UPDATES_ROLLUP_ID)
    expect(rollup.kind).toBe('Updates')
    expect(rollup.severity).toBe('incident') // mirrors the critical update alert
    expect(rollup.name).toBe('7 critical updates')
    expect(rollup.nav.route).toEqual({ name: 'updates' })
  })

  it('shows no updates entry when there is no update alert, even if the scanner count is positive', () => {
    expect(buildAttentionItems([], [], 7, NOW)).toEqual([])
  })

  it('falls back to the update-alert count when the scanner count is unavailable', () => {
    const items = buildAttentionItems([], [updateAlert('u1')], 0, NOW)
    expect(items[0]!.name).toBe('1 critical update')
  })

  it('dedupes a warning monitor with a critical alert on the same entity, keeping the worst severity and monitor nav', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:x', type: 'container', name: 'x', status: 'warning' })],
      [alert({ severity: 'critical', entity_type: 'container', entity_id: 'x', entity_name: 'x' })],
      0,
      NOW,
    )
    expect(items).toHaveLength(1)
    expect(items[0]!.severity).toBe('incident')
    expect(items[0]!.nav.slideOver).toEqual({ type: 'container', id: 'x' })
    expect(items[0]!.kind).toBe('Container')
  })

  it('sorts incident before warning, then most recent first', () => {
    const items = buildAttentionItems(
      [
        monitor({ id: 'container:old-inc', type: 'container', name: 'old-inc', status: 'down', updatedAt: '2026-06-13T09:00:00Z' }),
        monitor({ id: 'container:new-inc', type: 'container', name: 'new-inc', status: 'down', updatedAt: '2026-06-13T11:50:00Z' }),
        monitor({ id: 'endpoint:warn', type: 'endpoint', name: 'warn', status: 'warning' }),
      ],
      [],
      0,
      NOW,
    )
    expect(items.map((i) => i.name)).toEqual(['new-inc', 'old-inc', 'warn'])
  })

  it('formats relative timestamps from now', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:a', type: 'container', name: 'a', status: 'down', updatedAt: '2026-06-13T11:33:00Z' })],
      [],
      0,
      NOW,
    )
    expect(items[0]!.timestamp).toBe('27 min')
  })
})
