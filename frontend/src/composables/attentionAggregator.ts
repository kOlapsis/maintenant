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

import type { RouteLocationRaw } from 'vue-router'
import type { UnifiedMonitor, MonitorType } from '@/stores/dashboard'
import type { Agent } from '@/services/agentApi'
import type { Alert } from '@/services/alertApi'
import { severityFromStatus, severityFromAlert, severityRank, type Severity } from '@/composables/useSeverity'
import type { EntityType } from '@/composables/useDetailSlideOver'

// The local runtime sentinel — never an attention item when "disconnected".
export const LOCAL_AGENT = '00000000-0000-0000-0000-000000000000'
const SLIDEOVER_TYPES = new Set<string>(['container', 'heartbeat', 'certificate'])

const KIND: Record<MonitorType, string> = {
  container: 'Container',
  endpoint: 'Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'Certificate',
  workload: 'Workload',
}

// Display kind for an engine alert, keyed by its source. Security findings read
// as "Security" regardless of the underlying entity (a security alert targets a
// container but is not a container-down incident). Falls back to a capitalised
// source so a new alert source still renders something sane.
const ALERT_SOURCE_KIND: Record<string, string> = {
  security: 'Security',
  agent: 'Agent',
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

export type MonitorLike = Pick<
  UnifiedMonitor,
  'id' | 'type' | 'name' | 'status' | 'statusLabel' | 'subtitle' | 'updatedAt' | 'link'
>
export type AgentLike = Pick<
  Agent,
  'agent_id' | 'hostname' | 'label' | 'status' | 'connection_state' | 'last_seen_at'
>
export type AlertLike = Pick<
  Alert,
  'id' | 'source' | 'alert_type' | 'severity' | 'status' | 'entity_type' | 'entity_id' | 'entity_name' | 'message' | 'fired_at'
>

export function relTime(input: string | null | undefined, now: number): { ts: number; label: string } {
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

function capitalise(s: string): string {
  return s ? s[0]!.toUpperCase() + s.slice(1) : s
}

// Single source of truth for the unified attention list, built from the three
// GLOBALLY-available signals (monitors + disconnected agents + engine alerts).
// Entities are deduplicated by `${entityType}:${entityId}` with incident
// outranking warning, so a warning monitor that also has a critical alert counts
// once, as an incident, keeping the monitor's rich nav. `paused`/`unknown`
// monitors and `info`/resolved alerts are never actionable and are skipped.
// Updates are intentionally NOT a source here — they are warning-only and
// dashboard-scoped (see buildAttentionItems).
export function buildUnifiedAttention(
  monitors: MonitorLike[],
  alerts: AlertLike[],
  agents: AgentLike[],
  now: number,
): AttentionItem[] {
  const byKey = new Map<string, AttentionItem>()

  for (const m of monitors) {
    const severity = severityFromStatus(m.status)
    if (severity !== 'incident' && severity !== 'warning') continue
    const { ts, label } = relTime(m.updatedAt, now)
    const entityId = m.id.split(':')[1] ?? ''
    byKey.set(m.id, {
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

  for (const a of agents) {
    if (a.agent_id === LOCAL_AGENT || a.status !== 'active' || a.connection_state !== 'disconnected') {
      continue
    }
    const { ts, label } = relTime(a.last_seen_at, now)
    byKey.set(`agent:${a.agent_id}`, {
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

  for (const al of alerts) {
    if (al.status !== 'active') continue
    const severity = severityFromAlert(al.severity)
    if (severity !== 'incident' && severity !== 'warning') continue
    const key = `${al.entity_type}:${al.entity_id}`
    const existing = byKey.get(key)
    if (existing) {
      // Same entity already represented by a monitor/agent — upgrade severity if
      // the alert is more severe, but keep the existing item's richer nav/kind.
      if (severityRank(severity) < severityRank(existing.severity)) existing.severity = severity
      continue
    }
    const { ts, label } = relTime(al.fired_at, now)
    byKey.set(key, {
      id: key,
      severity,
      name: al.entity_name || al.entity_id,
      kind: ALERT_SOURCE_KIND[al.source] ?? capitalise(al.source),
      description: al.message,
      ts,
      timestamp: label,
      nav:
        SLIDEOVER_TYPES.has(al.entity_type) && al.entity_id
          ? { slideOver: { type: al.entity_type as EntityType, id: al.entity_id } }
          : al.entity_type === 'agent'
            ? { route: { name: 'agents' } }
            : { route: { name: 'alerts' } },
    })
  }

  return Array.from(byKey.values())
}
