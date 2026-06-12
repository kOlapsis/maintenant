// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { computed } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import { useDashboardStore, type UnifiedMonitor, type MonitorType } from '@/stores/dashboard'
import { useUpdatesStore } from '@/stores/updates'
import { useAgentsStore } from '@/stores/agents'
import type { ImageUpdate } from '@/services/updateApi'
import type { Agent } from '@/services/agentApi'
import { severityFromStatus, severityRank, type Severity } from '@/composables/useSeverity'
import type { EntityType } from '@/composables/useDetailSlideOver'

// The local runtime sentinel — never an attention item when "disconnected".
const LOCAL_AGENT = '00000000-0000-0000-0000-000000000000'
const SLIDEOVER_TYPES = new Set<string>(['container', 'heartbeat', 'certificate'])

const KIND: Record<MonitorType, string> = {
  container: 'Container',
  endpoint: 'Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'Certificate',
  workload: 'Workload',
}

export interface AttentionItem {
  id: string
  severity: Severity
  name: string
  kind: string
  description: string
  /** Epoch ms for recency sort (0 when unknown). */
  ts: number
  /** Relative display, e.g. "27 min". */
  timestamp: string
  nav: { slideOver?: { type: EntityType; id: string }; route?: RouteLocationRaw }
}

function relTime(input: string | null | undefined, now: number): { ts: number; label: string } {
  if (!input) return { ts: 0, label: '' }
  const t = Date.parse(input)
  if (Number.isNaN(t)) return { ts: 0, label: '' }
  const s = Math.max(0, Math.floor((now - t) / 1000))
  if (s < 60) return { ts: t, label: `${s}s` }
  const m = Math.floor(s / 60)
  if (m < 60) return { ts: t, label: `${m} min` }
  const h = Math.floor(m / 60)
  if (h < 24) return { ts: t, label: `${h} h` }
  return { ts: t, label: `${Math.floor(h / 24)} d` }
}

type MonitorLike = Pick<
  UnifiedMonitor,
  'id' | 'type' | 'name' | 'status' | 'statusLabel' | 'subtitle' | 'updatedAt' | 'link'
>
type UpdateLike = Pick<
  ImageUpdate,
  'id' | 'container_name' | 'update_type' | 'status' | 'current_tag' | 'latest_tag' | 'detected_at'
>
type AgentLike = Pick<
  Agent,
  'agent_id' | 'hostname' | 'label' | 'status' | 'connection_state' | 'last_seen_at'
>

// Pure aggregation — unit-tested. Merges unhealthy monitors, critical updates and
// disconnected agents into one severity-sorted attention list. `paused`/`unknown`
// monitors are never actionable, so they are skipped.
export function buildAttentionItems(
  monitors: MonitorLike[],
  updates: UpdateLike[],
  agents: AgentLike[],
  now: number,
): AttentionItem[] {
  const out: AttentionItem[] = []

  for (const m of monitors) {
    const severity = severityFromStatus(m.status)
    if (severity !== 'incident' && severity !== 'warning') continue
    const { ts, label } = relTime(m.updatedAt, now)
    const entityId = m.id.split(':')[1] ?? ''
    out.push({
      id: m.id,
      severity,
      name: m.name,
      kind: KIND[m.type] ?? m.type,
      description: m.subtitle ? `${m.statusLabel} — ${m.subtitle}` : m.statusLabel,
      ts,
      timestamp: label,
      nav:
        SLIDEOVER_TYPES.has(m.type) && entityId
          ? { slideOver: { type: m.type as EntityType, id: entityId } }
          : { route: m.link },
    })
  }

  for (const u of updates) {
    if (u.update_type !== 'critical' || u.status === 'pinned') continue
    const { ts, label } = relTime(u.detected_at, now)
    out.push({
      id: `update:${u.id}`,
      severity: 'warning',
      name: u.container_name,
      kind: 'Update',
      description: `Critical update available — ${u.current_tag} → ${u.latest_tag}`,
      ts,
      timestamp: label,
      nav: { route: { name: 'updates' } },
    })
  }

  for (const a of agents) {
    if (a.agent_id === LOCAL_AGENT || a.status !== 'active' || a.connection_state !== 'disconnected') {
      continue
    }
    const { ts, label } = relTime(a.last_seen_at, now)
    out.push({
      id: `agent:${a.agent_id}`,
      severity: 'incident',
      name: a.label || a.hostname,
      kind: 'Agent',
      description: 'Agent disconnected',
      ts,
      timestamp: label,
      nav: { route: { name: 'agents' } },
    })
  }

  return out.sort((x, y) => severityRank(x.severity) - severityRank(y.severity) || y.ts - x.ts)
}

export function useAttentionItems() {
  const dashboard = useDashboardStore()
  const updates = useUpdatesStore()
  const agents = useAgentsStore()

  const items = computed(() =>
    buildAttentionItems(dashboard.monitors, updates.updates, agents.agents, Date.now()),
  )

  return { items }
}
