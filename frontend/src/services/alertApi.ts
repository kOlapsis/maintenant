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

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

// --- Types ---

export interface Alert {
  id: string
  source: string
  alert_type: string
  severity: string
  status: string
  message: string
  entity_type: string
  entity_id: string
  entity_name: string
  details?: Record<string, unknown>
  resolved_by_id?: string | null
  fired_at: string
  resolved_at?: string | null
  created_at: string
}

export interface ListAlertsParams {
  source?: string
  severity?: string
  status?: string
  before?: string
  limit?: number
}

export interface ListAlertsResponse {
  alerts: Alert[]
  has_more: boolean
}

export interface ActiveAlertsResponse {
  critical: Alert[]
  warning: Alert[]
  info: Alert[]
}

export interface NotificationChannel {
  id: string
  name: string
  type: string
  url: string
  headers: string
  enabled: boolean
  health: string
  created_at: string
  updated_at: string
}

export interface SilenceRule {
  id: string
  entity_type: string
  entity_id?: string | null
  source: string
  reason: string
  starts_at: string
  duration_seconds: number
  expires_at: string
  is_active: boolean
  cancelled_at?: string | null
  created_at: string
}

export interface CreateSilenceRuleInput {
  duration_seconds: number
  entity_type?: string
  entity_id?: string
  source?: string
  reason?: string
}

// --- Helpers ---

function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  return apiFetch<T>(url, init)
}

function fetchNoContent(url: string, init?: RequestInit): Promise<void> {
  return apiFetchVoid(url, init)
}

// --- Alerts ---

export function listAlerts(params?: ListAlertsParams): Promise<ListAlertsResponse> {
  const url = new URL(`${API_BASE}/alerts`, window.location.origin)
  if (params?.source) url.searchParams.set('source', params.source)
  if (params?.severity) url.searchParams.set('severity', params.severity)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.before) url.searchParams.set('before', params.before)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  return fetchJSON<ListAlertsResponse>(url.toString())
}

export function getActiveAlerts(): Promise<ActiveAlertsResponse> {
  return fetchJSON<ActiveAlertsResponse>(`${API_BASE}/alerts/active`)
}

export function getAlert(id: string): Promise<Alert> {
  return fetchJSON<Alert>(`${API_BASE}/alerts/${id}`)
}

// --- Channels ---

export function listChannels(): Promise<{ channels: NotificationChannel[] }> {
  return fetchJSON(`${API_BASE}/channels`)
}

export function createChannel(data: {
  name: string
  type?: string
  url: string
  headers?: string
  enabled: boolean
}): Promise<NotificationChannel> {
  return fetchJSON(`${API_BASE}/channels`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateChannel(
  id: string,
  data: Partial<{ name: string; type: string; url: string; headers: string; enabled: boolean }>,
): Promise<NotificationChannel> {
  return fetchJSON(`${API_BASE}/channels/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteChannel(id: string): Promise<void> {
  return fetchNoContent(`${API_BASE}/channels/${id}`, { method: 'DELETE' })
}

export function testChannel(id: string): Promise<{ status: string; response_code?: number; error?: string }> {
  return fetchJSON(`${API_BASE}/channels/${id}/test`, { method: 'POST' })
}

// --- Silence Rules ---

export function listSilenceRules(activeOnly?: boolean): Promise<{ rules: SilenceRule[] }> {
  const url = new URL(`${API_BASE}/silence`, window.location.origin)
  if (activeOnly) url.searchParams.set('active', 'true')
  return fetchJSON(url.toString())
}

export function createSilenceRule(data: CreateSilenceRuleInput): Promise<SilenceRule> {
  return fetchJSON(`${API_BASE}/silence`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function cancelSilenceRule(id: string): Promise<void> {
  return fetchNoContent(`${API_BASE}/silence/${id}`, { method: 'DELETE' })
}
