// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

export interface EscalationScope {
  kind: 'container' | 'endpoint' | 'heartbeat' | 'certificate' | 'monitor'
  ref_id: string
}

export interface EscalationFilters {
  severities: string[]
  scopes: EscalationScope[]
  tags: string[]
}

export interface EscalationLevel {
  order: number
  delay_seconds: number
  channel_ids: string[]
}

export interface EscalationPolicy {
  id: string
  name: string
  active: boolean
  filters: EscalationFilters
  levels: EscalationLevel[]
  created_at: string
  updated_at: string
}

export interface EscalationLimits {
  max_active: number
  max_levels: number
  current_active: number
}

export interface EscalationRun {
  id: string
  policy_id: string | null
  policy: { id: string | null; name: string } | null
  alert_id: string
  status: string
  last_executed_level_index: number
  started_at: string
  ended_at: string | null
  next_action_at: string | null
  deliveries_summary?: {
    sent: number
    failed: number
    pending: number
  }
}

export interface EscalationDelivery {
  id: string
  run_id: string
  level_index: number
  channel_id: string | null
  channel_name?: string
  status: string
  error?: string
  attempt_started_at: string
  sent_at: string | null
}

export interface PolicyRequest {
  name: string
  active: boolean
  filters: EscalationFilters
  levels: Array<{ delay_seconds: number; channel_ids: string[] }>
}

export interface OverlapWarning {
  policy_id: string
  policy_name: string
  shared_channels: number[]
  filter_intersection: string
}
