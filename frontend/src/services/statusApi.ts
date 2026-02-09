const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

// --- Types ---

export interface ComponentGroup {
  id: number
  name: string
  display_order: number
  component_count?: number
  created_at: string
}

export interface StatusComponent {
  id: number
  monitor_type: string
  monitor_id: number
  display_name: string
  group_id: number | null
  group_name?: string
  display_order: number
  visible: boolean
  status_override: string | null
  auto_incident: boolean
  derived_status: string
  effective_status: string
  created_at: string
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

// --- Component Groups ---

export function listGroups(): Promise<ComponentGroup[]> {
  return fetchJSON<ComponentGroup[]>(`${API_BASE}/status/groups`)
}

export function createGroup(data: { name: string; display_order?: number }): Promise<ComponentGroup> {
  return fetchJSON(`${API_BASE}/status/groups`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateGroup(id: number, data: Partial<{ name: string; display_order: number }>): Promise<ComponentGroup> {
  return fetchJSON(`${API_BASE}/status/groups/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteGroup(id: number): Promise<void> {
  return fetchNoContent(`${API_BASE}/status/groups/${id}`, { method: 'DELETE' })
}

// --- Status Components ---

export function listComponents(): Promise<StatusComponent[]> {
  return fetchJSON<StatusComponent[]>(`${API_BASE}/status/components`)
}

export function createComponent(data: {
  monitor_type: string
  monitor_id: number
  display_name: string
  group_id?: number | null
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
    display_name: string
    group_id: number | null
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
