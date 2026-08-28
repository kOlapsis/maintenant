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
import type { Alert } from '@/services/alertApi'
import { severityFromStatus, severityFromAlert, severityRank, type Severity } from '@/composables/useSeverity'
import type { EntityType } from '@/composables/useDetailSlideOver'

const SLIDEOVER_TYPES = new Set<string>(['container', 'heartbeat', 'certificate'])

// Stable id of the single rolled-up "N critical updates" attention entry.
export const UPDATES_ROLLUP_ID = 'updates:critical'

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

// Kind label for the rolled-up updates entry.
export const UPDATES_KIND = 'Updates'

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

// Single source of truth for the unified attention list. Severity is NEVER
// invented here: it comes straight from the alert engine (critical→incident,
// warning→warning) for alerts, and from the monitor's own status for monitors,
// so the dashboard counters always agree with the Alerts page. Entities are
// deduplicated by `${entityType}:${entityId}`, with the more severe of a
// monitor/alert pair kept. All critical/warning update alerts collapse into one
// "Updates" roll-up entry (they are a single maintenance concern, not N
// incidents) carrying the alerts' own severity.
export function buildUnifiedAttention(
  monitors: MonitorLike[],
  alerts: AlertLike[],
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

  // Update alerts collapse into one roll-up; track their count, worst severity
  // and most recent fire time as we go.
  let updateCount = 0
  let updateSeverity: Severity | null = null
  let updateTs = 0

  for (const al of alerts) {
    if (al.status !== 'active') continue
    const severity = severityFromAlert(al.severity)
    if (severity !== 'incident' && severity !== 'warning') continue

    if (al.source === 'update') {
      updateCount++
      if (updateSeverity === null || severityRank(severity) < severityRank(updateSeverity)) {
        updateSeverity = severity
      }
      updateTs = Math.max(updateTs, relTime(al.fired_at, now).ts)
      continue
    }

    const key = `${al.entity_type}:${al.entity_id}`
    const existing = byKey.get(key)
    if (existing) {
      // Same entity already represented by a monitor — keep the more severe of
      // the two, preserving the monitor's richer nav/kind.
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
        al.entity_type === 'agent'
          ? { route: { name: 'agents' } }
          : SLIDEOVER_TYPES.has(al.entity_type) && al.entity_id
            ? { slideOver: { type: al.entity_type as EntityType, id: al.entity_id } }
            : { route: { name: 'alerts' } },
    })
  }

  const out = Array.from(byKey.values())

  if (updateCount > 0 && updateSeverity !== null) {
    out.push({
      id: UPDATES_ROLLUP_ID,
      severity: updateSeverity,
      name: `${updateCount} critical update${updateCount > 1 ? 's' : ''}`,
      kind: UPDATES_KIND,
      description: 'Review available container updates',
      ts: updateTs,
      timestamp: '',
      nav: { route: { name: 'updates' } },
    })
  }

  return out
}
