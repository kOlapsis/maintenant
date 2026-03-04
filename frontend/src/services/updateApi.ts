// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
import { apiFetch, apiFetchVoid } from './apiFetch'

export interface ImageUpdate {
  id: number
  container_id: string
  container_name: string
  image: string
  current_tag: string
  current_digest: string
  latest_tag: string
  latest_digest: string
  update_type: string
  published_at: string | null
  changelog_url: string
  changelog_summary: string
  has_breaking_changes: boolean
  risk_score: number
  status: string
  detected_at: string
  pin_reason?: string
}

export interface UpdateSummary {
  last_scan: string
  next_scan: string
  scan_status: string
  counts: {
    critical: number
    recommended: number
    available: number
    up_to_date: number
    untracked: number
    pinned: number
  }
  cve_counts: {
    critical: number
    high: number
    medium: number
    low: number
  }
  host_risk_score: number
}

export interface ContainerUpdateDetail {
  container_id: string
  container_name: string
  image: string
  current_tag: string
  current_digest: string
  latest_tag: string
  latest_digest: string
  update_type: string
  risk_score: number
  active_cves: CVEInfo[]
  pinned: boolean
  pin_reason?: string
  source_url: string
  previous_digest: string
  update_command: string
  changelog_url: string
  changelog_summary: string
  has_breaking_changes: boolean
}

export interface CVEInfo {
  cve_id: string
  cvss_score: number
  severity: string
  summary: string
  fixed_in: string
  first_detected_at?: string
}

export interface CVEListResponse {
  cves: Array<{
    cve_id: string
    cvss_score: number
    severity: string
    summary: string
    affected_containers: Array<{
      container_id: string
      container_name: string
      image: string
      fixed_in: string
    }>
    first_detected_at: string
  }>
  total: number
  by_severity: Record<string, number>
}

export interface RiskScore {
  container_id: string
  container_name: string
  risk_score: number
  level: string
}

export interface RiskListResponse {
  containers: RiskScore[]
  host_risk_score: number
  host_risk_level: string
}

export interface ScanStatus {
  scan_id: number
  status: string
  started_at: string
  completed_at?: string
  containers_scanned: number
  updates_found: number
  errors: number
}

export interface Exclusion {
  id: number
  pattern: string
  pattern_type: string
  created_at: string
}

// Updates
export function fetchUpdates(filters?: { status?: string; update_type?: string; min_risk?: number }): Promise<{ updates: ImageUpdate[]; last_scan: string; next_scan: string }> {
  const params = new URLSearchParams()
  if (filters?.status) params.set('status', filters.status)
  if (filters?.update_type) params.set('update_type', filters.update_type)
  if (filters?.min_risk) params.set('min_risk', String(filters.min_risk))
  const qs = params.toString()
  return apiFetch(`${API_BASE}/updates${qs ? '?' + qs : ''}`)
}

export function fetchUpdateSummary(): Promise<UpdateSummary> {
  return apiFetch(`${API_BASE}/updates/summary`)
}

export function fetchContainerUpdate(containerId: string): Promise<ContainerUpdateDetail> {
  return apiFetch(`${API_BASE}/updates/container/${containerId}`)
}

export function triggerScan(): Promise<{ status: string; started_at: string }> {
  return apiFetch(`${API_BASE}/updates/scan`, { method: 'POST' })
}

export function fetchScanStatus(scanId: number): Promise<ScanStatus> {
  return apiFetch(`${API_BASE}/updates/scan/${scanId}`)
}

export function fetchDryRun(): Promise<{ would_update: Array<{ container_id: string; container_name: string; image: string; current_tag: string; latest_tag: string; update_type: string }> }> {
  return apiFetch(`${API_BASE}/updates/dry-run`)
}

export function fetchDigest(): Promise<any> {
  return apiFetch(`${API_BASE}/updates/digest`)
}

// Pinning
export function pinVersion(containerId: string, reason?: string): Promise<any> {
  return apiFetch(`${API_BASE}/updates/pin/${containerId}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason: reason || '' }),
  })
}

export function unpinVersion(containerId: string): Promise<void> {
  return apiFetchVoid(`${API_BASE}/updates/pin/${containerId}`, { method: 'DELETE' })
}

// Exclusions
export function fetchExclusions(): Promise<{ exclusions: Exclusion[] }> {
  return apiFetch(`${API_BASE}/updates/exclusions`)
}

export function addExclusion(pattern: string, patternType: string): Promise<Exclusion> {
  return apiFetch(`${API_BASE}/updates/exclusions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pattern, pattern_type: patternType }),
  })
}

export function removeExclusion(id: number): Promise<void> {
  return apiFetchVoid(`${API_BASE}/updates/exclusions/${id}`, { method: 'DELETE' })
}

// CVEs
export function fetchCves(filters?: { severity?: string; container_id?: string }): Promise<CVEListResponse> {
  const params = new URLSearchParams()
  if (filters?.severity) params.set('severity', filters.severity)
  if (filters?.container_id) params.set('container_id', filters.container_id)
  const qs = params.toString()
  return apiFetch(`${API_BASE}/cve${qs ? '?' + qs : ''}`)
}

export function fetchContainerCves(containerId: string): Promise<{ container_id: string; cves: CVEInfo[] }> {
  return apiFetch(`${API_BASE}/cve/${containerId}`)
}

// Risk
export function fetchRiskScores(): Promise<RiskListResponse> {
  return apiFetch(`${API_BASE}/risk`)
}

export function fetchContainerRisk(containerId: string): Promise<any> {
  return apiFetch(`${API_BASE}/risk/${containerId}`)
}

export function fetchRiskHistory(containerId: string, period: string = '7d'): Promise<{ container_id: string; history: Array<{ score: number; recorded_at: string }> }> {
  return apiFetch(`${API_BASE}/risk/${containerId}/history?period=${period}`)
}
