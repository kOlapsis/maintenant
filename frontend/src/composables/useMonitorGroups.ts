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

import { computed, type Component } from 'vue'
import { Box, Globe, Heart, Shield, Cloud } from 'lucide-vue-next'
import { useDashboardStore, type UnifiedMonitor, type MonitorType } from '@/stores/dashboard'
import { usePreferencesStore } from '@/stores/preferences'
import { severityFromStatus, severityRank, severityMeta, type Severity } from '@/composables/useSeverity'
import type { GridGroup, GridItem } from '@/components/ui/statusGrid'

const TYPE_META: Record<MonitorType, { label: string; icon: Component; kind: string }> = {
  container: { label: 'Containers', icon: Box, kind: 'Container' },
  endpoint: { label: 'HTTP Endpoints', icon: Globe, kind: 'Endpoint' },
  heartbeat: { label: 'Heartbeats', icon: Heart, kind: 'Heartbeat' },
  certificate: { label: 'SSL Certificates', icon: Shield, kind: 'SSL' },
  workload: { label: 'Workloads', icon: Cloud, kind: 'Workload' },
}
const TYPE_ORDER: MonitorType[] = ['container', 'workload', 'endpoint', 'certificate', 'heartbeat']

const SEVERITY_GROUPS: { severity: Severity; key: string }[] = [
  { severity: 'incident', key: 'incident' },
  { severity: 'warning', key: 'warning' },
  { severity: 'unknown', key: 'unknown' },
  { severity: 'ok', key: 'ok' },
  { severity: 'neutral', key: 'neutral' },
]

function toItem(m: UnifiedMonitor): GridItem {
  return {
    id: m.id,
    severity: severityFromStatus(m.status),
    name: m.name,
    meta: m.metricValue ? `${m.metricValue}${m.metricLabel ? ` ${m.metricLabel}` : ''}` : m.statusLabel,
    kind: TYPE_META[m.type].kind,
    description: m.subtitle,
    host: m.host ?? undefined,
  }
}

function isHealthy(items: GridItem[]): boolean {
  return items.every((i) => i.severity === 'ok' || i.severity === 'neutral')
}
function worstRank(items: GridItem[]): number {
  return items.reduce((min, i) => Math.min(min, severityRank(i.severity)), 99)
}
function bySeverityThenName(a: GridItem, b: GridItem): number {
  return severityRank(a.severity) - severityRank(b.severity) || a.name.localeCompare(b.name)
}

function groupByType(monitors: UnifiedMonitor[]): GridGroup[] {
  const buckets = new Map<MonitorType, GridItem[]>()
  for (const m of monitors) {
    const list = buckets.get(m.type) ?? []
    list.push(toItem(m))
    buckets.set(m.type, list)
  }
  const groups: GridGroup[] = []
  for (const type of TYPE_ORDER) {
    const items = buckets.get(type)
    if (!items || items.length === 0) continue
    items.sort(bySeverityThenName)
    groups.push({
      key: type,
      label: TYPE_META[type].label,
      icon: TYPE_META[type].icon,
      items,
      collapsedByDefault: isHealthy(items),
    })
  }
  // Problem groups float up; fully-healthy ones sink (and stay collapsed).
  return groups.sort((a, b) => worstRank(a.items) - worstRank(b.items))
}

function groupBySeverity(monitors: UnifiedMonitor[]): GridGroup[] {
  const buckets = new Map<Severity, GridItem[]>()
  for (const m of monitors) {
    const item = toItem(m)
    const list = buckets.get(item.severity) ?? []
    list.push(item)
    buckets.set(item.severity, list)
  }
  const groups: GridGroup[] = []
  for (const { severity, key } of SEVERITY_GROUPS) {
    const items = buckets.get(severity)
    if (!items || items.length === 0) continue
    items.sort((a, b) => a.name.localeCompare(b.name))
    groups.push({
      key,
      label: severityMeta(severity).label,
      icon: severityMeta(severity).icon,
      items,
      collapsedByDefault: severity === 'ok' || severity === 'neutral',
    })
  }
  return groups
}

export function useMonitorGroups() {
  const dashboard = useDashboardStore()
  const prefs = usePreferencesStore()

  const filtered = computed<UnifiedMonitor[]>(() => {
    const q = dashboard.searchQuery.toLowerCase().trim()
    if (!q) return dashboard.monitors
    return dashboard.monitors.filter(
      (m) =>
        m.name.toLowerCase().includes(q) ||
        m.subtitle.toLowerCase().includes(q) ||
        m.statusLabel.toLowerCase().includes(q),
    )
  })

  const monitorGroups = computed<GridGroup[]>(() =>
    prefs.monitorsGroupBy === 'severity' ? groupBySeverity(filtered.value) : groupByType(filtered.value),
  )

  return { monitorGroups }
}
