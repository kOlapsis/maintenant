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
import { severityRank } from '@/composables/useSeverity'
import {
  buildUnifiedAttention,
  type AttentionItem,
  type MonitorLike,
  type AgentLike,
  type AlertLike,
} from '@/composables/attentionAggregator'

export type { AttentionItem, MonitorLike, AgentLike, AlertLike }

// The roll-up kind for the single "X critical updates" entry. Kept in sync with
// the breakdown exclusion in DashboardPage so updates never inflate the verdict.
export const UPDATES_KIND = 'Updates'

// Pure aggregation — unit-tested. Wraps the canonical `buildUnifiedAttention`
// (monitors + agents + alerts) and, when there are critical container updates,
// appends ONE roll-up entry linking to the Updates page — never per-container
// items, which were fragile (update alerts share an empty entity id and collide)
// and misleading (only one ever survived). Final order: incidents first, then
// most recent.
export function buildAttentionItems(
  monitors: MonitorLike[],
  agents: AgentLike[],
  alerts: AlertLike[],
  criticalUpdateCount: number,
  now: number,
): AttentionItem[] {
  const out = buildUnifiedAttention(monitors, alerts, agents, now)

  if (criticalUpdateCount > 0) {
    out.push({
      id: 'updates:critical',
      severity: 'warning',
      name: `${criticalUpdateCount} critical update${criticalUpdateCount > 1 ? 's' : ''}`,
      kind: UPDATES_KIND,
      description: 'Review available container updates',
      ts: 0,
      timestamp: '',
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
    return buildAttentionItems(dashboard.monitors, agents.agents, allAlerts, updates.criticalCount, Date.now())
  })

  return { items }
}
