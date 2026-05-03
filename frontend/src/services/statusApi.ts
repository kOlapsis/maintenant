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

export interface MonitorRef {
  type: string   // 'container' | 'endpoint' | 'heartbeat' | 'certificate'
  id: number
  name?: string
  status?: string
}

export interface StatusComponent {
  id: number
  composition_mode: 'explicit' | 'match-all'
  monitors: MonitorRef[]
  match_all_type: string | null
  display_name: string
  display_order: number
  visible: boolean
  status_override: string | null
  auto_incident: boolean
  derived_status: string
  effective_status: string
  needs_attention?: boolean
  created_at: string
  updated_at?: string
}

export interface Incident {
  id: number
  title: string
  severity: string
  status: string
  components: IncidentComponentRef[]
  updates: IncidentUpdate[]
  created_at: string
  resolved_at: string | null
}

export interface IncidentComponentRef {
  component_id: number
  name: string
}

export interface IncidentUpdate {
  id: number
  incident_id: number
  status: string
  message: string
  created_at: string
}

export interface MaintenanceWindow {
  id: number
  title: string
  description: string
  starts_at: string
  ends_at: string
  active: boolean
  components: MaintenanceComponentRef[]
  created_at: string
}

export interface MaintenanceComponentRef {
  component_id: number
  name: string
}

export interface SubscriberListResponse {
  subscribers: MaskedSubscriber[]
  total: number
  confirmed: number
}

export interface MaskedSubscriber {
  id: number
  email: string
  confirmed: boolean
  created_at: string
}

// --- Helpers ---

function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  return apiFetch<T>(url, init)
}

function fetchNoContent(url: string, init?: RequestInit): Promise<void> {
  return apiFetchVoid(url, init)
}

// --- Status Components ---

export function listComponents(): Promise<StatusComponent[]> {
  return fetchJSON<StatusComponent[]>(`${API_BASE}/status/components`)
}

export function createComponent(data: {
  composition_mode: 'explicit' | 'match-all'
  monitors?: MonitorRef[]
  match_all_type?: string | null
  display_name: string
  visible?: boolean
  auto_incident?: boolean
}): Promise<StatusComponent> {
  return fetchJSON(`${API_BASE}/status/components`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateComponent(
  id: number,
  data: Partial<{
    monitors: MonitorRef[]
    display_name: string
    display_order: number
    visible: boolean
    status_override: string | null
    auto_incident: boolean
  }>,
): Promise<StatusComponent> {
  return fetchJSON(`${API_BASE}/status/components/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteComponent(id: number): Promise<void> {
  return fetchNoContent(`${API_BASE}/status/components/${id}`, { method: 'DELETE' })
}

// --- Incidents ---

export function listIncidents(params?: {
  status?: string
  severity?: string
  limit?: number
  offset?: number
}): Promise<{ incidents: Incident[]; total: number }> {
  const url = new URL(`${API_BASE}/status/incidents`, window.location.origin)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.severity) url.searchParams.set('severity', params.severity)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON(url.toString())
}

export function createIncident(data: {
  title: string
  severity: string
  status?: string
  component_ids: number[]
  message: string
}): Promise<Incident> {
  return fetchJSON(`${API_BASE}/status/incidents`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function postIncidentUpdate(
  incidentId: number,
  data: { status: string; message: string },
): Promise<IncidentUpdate> {
  return fetchJSON(`${API_BASE}/status/incidents/${incidentId}/updates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateIncident(
  id: number,
  data: Partial<{ title: string; severity: string; component_ids: number[] }>,
): Promise<Incident> {
  return fetchJSON(`${API_BASE}/status/incidents/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteIncident(id: number): Promise<void> {
  return fetchNoContent(`${API_BASE}/status/incidents/${id}`, { method: 'DELETE' })
}

// --- Maintenance Windows ---

export function listMaintenance(params?: { status?: string; limit?: number }): Promise<MaintenanceWindow[]> {
  const url = new URL(`${API_BASE}/status/maintenance`, window.location.origin)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  return fetchJSON(url.toString())
}

export function createMaintenance(data: {
  title: string
  description?: string
  starts_at: string
  ends_at: string
  component_ids: number[]
}): Promise<MaintenanceWindow> {
  return fetchJSON(`${API_BASE}/status/maintenance`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateMaintenance(
  id: number,
  data: Partial<{
    title: string
    description: string
    starts_at: string
    ends_at: string
    component_ids: number[]
  }>,
): Promise<MaintenanceWindow> {
  return fetchJSON(`${API_BASE}/status/maintenance/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteMaintenance(id: number): Promise<void> {
  return fetchNoContent(`${API_BASE}/status/maintenance/${id}`, { method: 'DELETE' })
}

// --- Subscribers ---

export function listSubscribers(): Promise<SubscriberListResponse> {
  return fetchJSON<SubscriberListResponse>(`${API_BASE}/status/subscribers`)
}

// --- SMTP Config ---

export interface SmtpConfig {
  host: string
  port: number
  username: string
  password?: string
  tls_policy: string
  from_address: string
  from_name: string
  configured: boolean
  password_set?: boolean
}

export function getSmtpConfig(): Promise<SmtpConfig> {
  return fetchJSON<SmtpConfig>(`${API_BASE}/status/smtp`)
}

export function updateSmtpConfig(data: Partial<SmtpConfig>): Promise<{ status: string }> {
  return fetchJSON(`${API_BASE}/status/smtp`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function testSmtp(): Promise<{ status: string; error?: string }> {
  return fetchJSON(`${API_BASE}/status/smtp/test`, { method: 'POST' })
}
