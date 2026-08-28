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
import { useAlertsStore } from '@/stores/alerts'
import { severityRank } from '@/composables/useSeverity'
import {
  buildUnifiedAttention,
  UPDATES_ROLLUP_ID,
  UPDATES_KIND,
  type AttentionItem,
  type MonitorLike,
  type AlertLike,
} from '@/composables/attentionAggregator'

export { UPDATES_KIND }
export type { AttentionItem, MonitorLike, AlertLike }

// Pure aggregation — unit-tested. Severities come straight from the alert engine
// and monitor statuses (see buildUnifiedAttention); this wrapper only relabels
// the updates roll-up with the scanner's reliable critical count and sorts the
// list incidents-first, then most recent.
export function buildAttentionItems(
  monitors: MonitorLike[],
  alerts: AlertLike[],
  criticalUpdateCount: number,
  now: number,
): AttentionItem[] {
  const out = buildUnifiedAttention(monitors, alerts, now)

  if (criticalUpdateCount > 0) {
    const rollup = out.find((i) => i.id === UPDATES_ROLLUP_ID)
    if (rollup) rollup.name = `${criticalUpdateCount} critical update${criticalUpdateCount > 1 ? 's' : ''}`
  }

  return out.sort((x, y) => severityRank(x.severity) - severityRank(y.severity) || y.ts - x.ts)
}

export function useAttentionItems() {
  const dashboard = useDashboardStore()
  const updates = useUpdatesStore()
  const alerts = useAlertsStore()

  const items = computed(() => {
    const active = alerts.activeAlerts
    const allAlerts = [...active.critical, ...active.warning, ...active.info]
    return buildAttentionItems(dashboard.monitors, allAlerts, updates.criticalCount, Date.now())
  })

  return { items }
}