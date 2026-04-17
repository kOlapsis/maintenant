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

export type CertStatus = 'valid' | 'expiring' | 'expired' | 'error' | 'unknown'
export type CertSource = 'auto' | 'standalone'

export interface CertMonitor {
  id: number
  hostname: string
  port: number
  source: CertSource
  endpoint_id?: number
  status: CertStatus
  check_interval_seconds: number
  warning_thresholds: number[]
  last_alerted_threshold?: number
  last_check_at?: string
  last_error?: string
  created_at: string
  latest_check?: CertCheckResult
}

export interface CertCheckResult {
  id: number
  subject_cn: string
  issuer_cn: string
  issuer_org: string
  sans: string[]
  serial_number: string
  signature_algorithm: string
  not_before?: string
  not_after?: string
  days_remaining: number
  chain_valid?: boolean
  chain_error?: string
  hostname_match?: boolean
  error_message?: string
  checked_at: string
  chain?: CertChainEntry[]
}

export interface CertChainEntry {
  position: number
  subject_cn: string
  issuer_cn: string
  not_before: string
  not_after: string
}

export interface CreateCertificateInput {
  hostname: string
  port?: number
  check_interval_seconds?: number
  warning_thresholds?: number[]
}

export interface UpdateCertificateInput {
  check_interval_seconds?: number
  warning_thresholds?: number[]
}

export interface CertificatesResponse {
  certificates: CertMonitor[]
  total: number
}

export interface CertificateDetailResponse {
  certificate: CertMonitor
  latest_check?: CertCheckResult
}

export interface CertificateCreateResponse {
  certificate: CertMonitor
  latest_check?: CertCheckResult
}

export interface ChecksResponse {
  monitor_id: number
  checks: CertCheckResult[]
  total: number
  has_more: boolean
}

function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  return apiFetch<T>(url, init)
}

export function listCertificates(params?: { status?: string; source?: string }): Promise<CertificatesResponse> {
  const url = new URL(`${API_BASE}/certificates`, window.location.origin)
  if (params?.status) url.searchParams.set('status', params.status)
  if (params?.source) url.searchParams.set('source', params.source)
  return fetchJSON<CertificatesResponse>(url.toString())
}

export function getCertificate(id: number): Promise<CertificateDetailResponse> {
  return fetchJSON<CertificateDetailResponse>(`${API_BASE}/certificates/${id}`)
}

export function createCertificate(data: CreateCertificateInput): Promise<CertificateCreateResponse> {
  return fetchJSON<CertificateCreateResponse>(`${API_BASE}/certificates`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function updateCertificate(id: number, data: UpdateCertificateInput): Promise<{ certificate: CertMonitor }> {
  return fetchJSON<{ certificate: CertMonitor }>(`${API_BASE}/certificates/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export function deleteCertificate(id: number): Promise<void> {
  return fetchJSON<void>(`${API_BASE}/certificates/${id}`, { method: 'DELETE' })
}

export function listChecks(id: number, params?: { limit?: number; offset?: number }): Promise<ChecksResponse> {
  const url = new URL(`${API_BASE}/certificates/${id}/checks`, window.location.origin)
  if (params?.limit) url.searchParams.set('limit', String(params.limit))
  if (params?.offset) url.searchParams.set('offset', String(params.offset))
  return fetchJSON<ChecksResponse>(url.toString())
}
