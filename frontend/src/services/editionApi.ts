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

import { apiFetch } from './apiFetch'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export interface QuotaEntry {
  used: number
  limit: number
}

export type QuotaResource = 'endpoints' | 'heartbeats' | 'certificates' | 'status_groups' | 'status_components'

export interface EditionResponse {
  edition: string
  organisation_name: string
  features: Record<string, boolean>
  quotas?: Partial<Record<QuotaResource, QuotaEntry>>
}

export function fetchEdition(): Promise<EditionResponse> {
  return apiFetch(`${API_BASE}/edition`)
}

export interface LicenseStatus {
  status: string
  plan: string
  message: string
  verified_at: string
  expires_at: string
}

export function fetchLicenseStatus(): Promise<LicenseStatus> {
  return apiFetch(`${API_BASE}/license/status`)
}
