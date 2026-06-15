// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { buildAttentionItems } from '../useAttentionItems'
import type { UnifiedMonitor } from '@/stores/dashboard'
import type { ImageUpdate } from '@/services/updateApi'
import type { Agent } from '@/services/agentApi'
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
    id: `alert-${p.entity_id}`,
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
      [],
      [],
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
      [],
      [],
      NOW,
    )
    expect(items.find((i) => i.name === 'a')!.nav.slideOver).toEqual({ type: 'container', id: 'a' })
    expect(items.find((i) => i.name === 'w')!.nav.route).toBeDefined()
  })

  it('includes critical non-pinned updates only', () => {
    const u = (p: Partial<ImageUpdate>): ImageUpdate => ({
      id: 'u', container_id: 'c', container_name: 'svc', image: '', current_tag: '1', current_digest: '',
      latest_tag: '2', latest_digest: '', update_type: 'critical', published_at: null, changelog_url: '',
      changelog_summary: '', has_breaking_changes: false, risk_score: 0, status: 'available',
      detected_at: '2026-06-13T11:00:00Z', ...p,
    })
    const items = buildAttentionItems(
      [],
      [u({ id: 'crit' }), u({ id: 'pinned', status: 'pinned' }), u({ id: 'low', update_type: 'available' })],
      [],
      [],
      NOW,
    )
    expect(items.map((i) => i.id)).toEqual(['update:crit'])
    expect(items[0]!.severity).toBe('warning')
  })

  it('includes disconnected active agents, excluding local and revoked', () => {
    const a = (p: Partial<Agent>): Agent => ({
      agent_id: 'x', hostname: 'host', label: '', os_arch: '', agent_version: '',
      detected_runtime: 'docker', status: 'active', connection_state: 'disconnected',
      last_seen_at: '2026-06-13T10:00:00Z', created_at: '', revoked_at: null, revoked_by: null, ...p,
    })
    const items = buildAttentionItems(
      [],
      [],
      [
        a({ agent_id: 'remote', hostname: 'remote' }),
        a({ agent_id: '00000000-0000-0000-0000-000000000000', hostname: 'local' }),
        a({ agent_id: 'revoked', status: 'revoked' }),
        a({ agent_id: 'online', connection_state: 'connected' }),
      ],
      [],
      NOW,
    )
    expect(items.map((i) => i.id)).toEqual(['agent:remote'])
    expect(items[0]!.severity).toBe('incident')
  })

  it('sorts incident before warning, then most recent first', () => {
    const items = buildAttentionItems(
      [
        monitor({ id: 'container:old-inc', type: 'container', name: 'old-inc', status: 'down', updatedAt: '2026-06-13T09:00:00Z' }),
        monitor({ id: 'container:new-inc', type: 'container', name: 'new-inc', status: 'down', updatedAt: '2026-06-13T11:50:00Z' }),
        monitor({ id: 'endpoint:warn', type: 'endpoint', name: 'warn', status: 'warning' }),
      ],
      [],
      [],
      [],
      NOW,
    )
    expect(items.map((i) => i.name)).toEqual(['new-inc', 'old-inc', 'warn'])
  })

  it('formats relative timestamps from now', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:a', type: 'container', name: 'a', status: 'down', updatedAt: '2026-06-13T11:33:00Z' })],
      [],
      [],
      [],
      NOW,
    )
    expect(items[0]!.timestamp).toBe('27 min')
  })

  it('surfaces critical security alerts as visible incidents with kind Security', () => {
    const items = buildAttentionItems(
      [],
      [],
      [],
      [alert({ severity: 'critical', entity_type: 'container', entity_id: 'traefik', entity_name: 'traefik' })],
      NOW,
    )
    expect(items).toHaveLength(1)
    expect(items[0]!.severity).toBe('incident')
    expect(items[0]!.kind).toBe('Security')
    // Container-scoped alert opens the container slide-over.
    expect(items[0]!.nav.slideOver).toEqual({ type: 'container', id: 'traefik' })
  })

  it('maps alert severity and drops non-actionable info alerts', () => {
    const items = buildAttentionItems(
      [],
      [],
      [],
      [
        alert({ severity: 'warning', entity_type: 'container', entity_id: 'w', entity_name: 'w' }),
        alert({ severity: 'info', entity_type: 'container', entity_id: 'i', entity_name: 'i' }),
      ],
      NOW,
    )
    expect(items.map((i) => i.severity)).toEqual(['warning'])
  })

  it('dedupes a warning monitor with a critical alert on the same entity, upgrading to incident and keeping monitor nav', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:x', type: 'container', name: 'x', status: 'warning' })],
      [],
      [],
      [alert({ severity: 'critical', entity_type: 'container', entity_id: 'x', entity_name: 'x' })],
      NOW,
    )
    expect(items).toHaveLength(1)
    expect(items[0]!.severity).toBe('incident')
    // Monitor representation wins the nav and kind.
    expect(items[0]!.nav.slideOver).toEqual({ type: 'container', id: 'x' })
    expect(items[0]!.kind).toBe('Container')
  })

  it('dedupes a down monitor with a warning alert on the same entity, staying an incident', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:x', type: 'container', name: 'x', status: 'down' })],
      [],
      [],
      [alert({ severity: 'warning', entity_type: 'container', entity_id: 'x', entity_name: 'x' })],
      NOW,
    )
    expect(items).toHaveLength(1)
    expect(items[0]!.severity).toBe('incident')
  })

  it('surfaces an orphan agent alert (no agent record) as an incident routed to agents', () => {
    const items = buildAttentionItems(
      [],
      [],
      [],
      [alert({ source: 'agent', alert_type: 'disconnected', severity: 'critical', entity_type: 'agent', entity_id: 'gone', entity_name: 'gone' })],
      NOW,
    )
    expect(items).toHaveLength(1)
    expect(items[0]!.severity).toBe('incident')
    expect(items[0]!.kind).toBe('Agent')
    expect(items[0]!.nav.route).toEqual({ name: 'agents' })
  })

  it('does not double-count an agent present both in the agent list and as an alert', () => {
    const a: Agent = {
      agent_id: 'dup', hostname: 'dup', label: '', os_arch: '', agent_version: '',
      detected_runtime: 'docker', status: 'active', connection_state: 'disconnected',
      last_seen_at: '2026-06-13T10:00:00Z', created_at: '', revoked_at: null, revoked_by: null,
    }
    const items = buildAttentionItems(
      [],
      [],
      [a],
      [alert({ source: 'agent', alert_type: 'disconnected', severity: 'critical', entity_type: 'agent', entity_id: 'dup', entity_name: 'dup' })],
      NOW,
    )
    expect(items.filter((i) => i.id === 'agent:dup')).toHaveLength(1)
    expect(items).toHaveLength(1)
  })
})
