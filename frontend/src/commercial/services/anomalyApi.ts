// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { apiFetch } from '@/services/apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export type ScopeType = 'container' | 'host'
export type SeriesState = 'learning' | 'ready' | 'relearning' | 'disabled'
export type Detector = 'spike' | 'drift' | 'change_point'
export type Tier = 'passive' | 'active'

export interface ScopeSummary {
  learning: number
  ready: number
  relearning: number
  active_anomalies: number
  progress: number
  earliest_ready_at: number | null
}

export type AnomalySummary = Record<string, ScopeSummary>

export interface SeriesItem {
  scope_type: ScopeType
  scope_id: string
  metric: string
  dimension: string
  node_id: string
  state: SeriesState
  progress: number
  sensitivity: string
  metrics: string[]
  current_score: number
  ready_at: number | null
  estimated_ready_at: number | null
}

export interface BaselinePoint {
  timestamp: number
  median: number
  lower: number
  upper: number
}

export interface BaselineBand {
  metric: string
  dimension: string
  points: BaselinePoint[]
}

export interface AnomalyEventItem {
  id: string
  scope_type: ScopeType
  scope_id: string
  metric: string
  dimension: string
  node_id: string
  detector: Detector
  tier: Tier
  started_at: number
  ended_at: number | null
  peak_value: number
  baseline_median: number
  peak_deviation: number
  alert_id?: string
  suppressed_by?: string
}

export function getAnomalySummary(): Promise<AnomalySummary> {
  return apiFetch<AnomalySummary>(`${API_BASE}/anomaly/summary`)
}

export function listAnomalySeries(scopeType?: ScopeType): Promise<SeriesItem[]> {
  const url = new URL(`${API_BASE}/anomaly/series`, window.location.origin)
  if (scopeType) url.searchParams.set('scope_type', scopeType)
  return apiFetch<{ series: SeriesItem[] }>(url.toString()).then((r) => r.series)
}

export function getSeriesDetail(scopeType: ScopeType, scopeId: string): Promise<SeriesItem[]> {
  const path = `${API_BASE}/anomaly/series/${scopeType}/${encodePath(scopeId)}`
  return apiFetch<{ metrics: SeriesItem[] }>(path).then((r) => r.metrics)
}

export interface AnomalySettings {
  bucket_pull: number
  min_bucket_pull: number
  max_bucket_pull: number
  min_samples: number
  alert_severity: string
  updated_at: number
}

export function getAnomalySettings(): Promise<AnomalySettings> {
  return apiFetch<AnomalySettings>(`${API_BASE}/anomaly/settings`)
}

export function updateAnomalySettings(bucketPull: number): Promise<AnomalySettings> {
  return apiFetch<AnomalySettings>(`${API_BASE}/anomaly/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bucket_pull: bucketPull }),
  })
}

export interface BaselineQuery {
  dimension?: string
  from?: number
  to?: number
}

export function getAnomalyBaseline(
  scopeType: ScopeType,
  scopeId: string,
  metric: string,
  opts: BaselineQuery = {},
): Promise<BaselineBand> {
  const url = new URL(`${API_BASE}/anomaly/baseline/${scopeType}/${encodePath(scopeId)}`, window.location.origin)
  url.searchParams.set('metric', metric)
  if (opts.dimension) url.searchParams.set('dimension', opts.dimension)
  if (opts.from != null) url.searchParams.set('from', String(opts.from))
  if (opts.to != null) url.searchParams.set('to', String(opts.to))
  return apiFetch<BaselineBand>(url.toString())
}

export interface ListEventsParams {
  scope_type?: ScopeType
  scope_id?: string
  node_id?: string
  metric?: string
  tier?: Tier
  active?: boolean
  limit?: number
}

export function listAnomalyEvents(params: ListEventsParams = {}): Promise<AnomalyEventItem[]> {
  const url = new URL(`${API_BASE}/anomaly/events`, window.location.origin)
  if (params.scope_type) url.searchParams.set('scope_type', params.scope_type)
  if (params.scope_id) url.searchParams.set('scope_id', params.scope_id)
  if (params.node_id) url.searchParams.set('node_id', params.node_id)
  if (params.metric) url.searchParams.set('metric', params.metric)
  if (params.tier) url.searchParams.set('tier', params.tier)
  if (params.active != null) url.searchParams.set('active', String(params.active))
  if (params.limit) url.searchParams.set('limit', String(params.limit))
  return apiFetch<{ events: AnomalyEventItem[] }>(url.toString()).then((r) => r.events)
}

// A scope id holds slashes the backend wildcard captures, so each segment is encoded on its own.
function encodePath(scopeId: string): string {
  return scopeId
    .split('/')
    .map((s) => encodeURIComponent(s))
    .join('/')
}
