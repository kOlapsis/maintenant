// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { buildAttentionItems } from '../useAttentionItems'
import type { UnifiedMonitor } from '@/stores/dashboard'
import type { ImageUpdate } from '@/services/updateApi'
import type { Agent } from '@/services/agentApi'

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
      NOW,
    )
    expect(items.map((i) => i.name)).toEqual(['new-inc', 'old-inc', 'warn'])
  })

  it('formats relative timestamps from now', () => {
    const items = buildAttentionItems(
      [monitor({ id: 'container:a', type: 'container', name: 'a', status: 'down', updatedAt: '2026-06-13T11:33:00Z' })],
      [],
      [],
      NOW,
    )
    expect(items[0]!.timestamp).toBe('27 min')
  })
})
