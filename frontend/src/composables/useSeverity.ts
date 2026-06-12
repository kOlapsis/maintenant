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

import type { Component } from 'vue'
import CheckIcon from '@/components/ui/icons/CheckIcon.vue'
import WarningIcon from '@/components/ui/icons/WarningIcon.vue'
import CrossIcon from '@/components/ui/icons/CrossIcon.vue'
import PauseIcon from '@/components/ui/icons/PauseIcon.vue'
import QuestionIcon from '@/components/ui/icons/QuestionIcon.vue'

// Visual severity scale for the design system. This is a PRESENTATION concept:
// it never mutates UnifiedStatus or any wire/store value. `incident` is the
// single red bucket (monitor `down` + critical alerts); `neutral` covers paused.
export type Severity = 'ok' | 'warning' | 'incident' | 'unknown' | 'neutral'

// Mirror of the store's UnifiedStatus union (kept local to avoid a store import
// in a pure presentation helper). Anything outside the map degrades to unknown.
type StatusLike = 'ok' | 'warning' | 'down' | 'paused' | 'unknown' | (string & {})
type AlertSeverityLike = 'critical' | 'warning' | 'info' | (string & {})

const STATUS_TO_SEVERITY: Record<string, Severity> = {
  ok: 'ok',
  warning: 'warning',
  down: 'incident',
  paused: 'neutral',
  unknown: 'unknown',
}

const ALERT_TO_SEVERITY: Record<string, Severity> = {
  critical: 'incident',
  warning: 'warning',
  info: 'neutral',
}

export function severityFromStatus(status: StatusLike): Severity {
  return STATUS_TO_SEVERITY[status] ?? 'unknown'
}

export function severityFromAlert(severity: AlertSeverityLike): Severity {
  return ALERT_TO_SEVERITY[severity] ?? 'neutral'
}

interface SeverityMeta {
  label: string
  icon: Component
  // Sort rank, problems-first. Mirrors the store's statusOrder so groups and
  // attention lists order identically: incident < warning < unknown < ok < neutral.
  rank: number
}

const META: Record<Severity, SeverityMeta> = {
  incident: { label: 'Incident', icon: CrossIcon, rank: 0 },
  warning: { label: 'Warning', icon: WarningIcon, rank: 1 },
  unknown: { label: 'Unknown', icon: QuestionIcon, rank: 2 },
  ok: { label: 'Operational', icon: CheckIcon, rank: 3 },
  neutral: { label: 'Paused', icon: PauseIcon, rank: 4 },
}

export function severityMeta(severity: Severity): SeverityMeta {
  return META[severity]
}

export function severityRank(severity: Severity): number {
  return META[severity].rank
}

// CSS custom-property reference for a severity, by part. Keeps every status
// colour tokenised — components bind these in :style, never a hardcoded hex.
type SeverityPart = 'solid' | 'bg' | 'text' | 'border'

export function severityVar(severity: Severity, part: SeverityPart = 'solid'): string {
  const suffix = part === 'solid' ? '' : `-${part}`
  return `var(--pb-sev-${severity}${suffix})`
}

// `incident` and `warning` are the only severities that demand attention.
export function isActionable(severity: Severity): boolean {
  return severity === 'incident' || severity === 'warning'
}
