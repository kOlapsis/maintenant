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
import { useDashboardStore } from '@/stores/dashboard'
import { useUpdatesStore } from '@/stores/updates'
import { useAgentsStore } from '@/stores/agents'
import { useAlertsStore } from '@/stores/alerts'
import type { ImageUpdate } from '@/services/updateApi'
import { severityRank } from '@/composables/useSeverity'
import {
  buildUnifiedAttention,
  relTime,
  type AttentionItem,
  type MonitorLike,
  type AgentLike,
  type AlertLike,
} from '@/composables/attentionAggregator'

export type { AttentionItem, MonitorLike, AgentLike, AlertLike }

type UpdateLike = Pick<
  ImageUpdate,
  'id' | 'container_name' | 'update_type' | 'status' | 'current_tag' | 'latest_tag' | 'detected_at'
>

// Pure aggregation — unit-tested. Wraps the canonical `buildUnifiedAttention`
// (monitors + agents + alerts) and appends critical container updates as a
// warning-only, dashboard-scoped source. Updates have no dedupable entity key,
// so they are concatenated as-is. Final order: incidents first, then most recent.
export function buildAttentionItems(
  monitors: MonitorLike[],
  updates: UpdateLike[],
  agents: AgentLike[],
  alerts: AlertLike[],
  now: number,
): AttentionItem[] {
  const out = buildUnifiedAttention(monitors, alerts, agents, now)

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

  return out.sort((x, y) => severityRank(x.severity) - severityRank(y.severity) || y.ts - x.ts)
}

export function useAttentionItems() {
  const dashboard = useDashboardStore()
  const updates = useUpdatesStore()
  const agents = useAgentsStore()
  const alerts = useAlertsStore()

  const items = computed(() => {
    const active = alerts.activeAlerts
    const allAlerts = [...active.critical, ...active.warning, ...active.info]
    return buildAttentionItems(dashboard.monitors, updates.updates, agents.agents, allAlerts, Date.now())
  })

  return { items }
}
