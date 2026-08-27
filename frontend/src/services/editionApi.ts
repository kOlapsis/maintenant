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

export type QuotaResource =
  | 'endpoints'
  | 'heartbeats'
  | 'certificates'
  | 'status_components'
  | 'agent_hosts'

/**
 * The known editions, ordered Community < Personal < Pro.
 *
 * The `(string & {})` arm is deliberate: a newer engine may report an edition
 * this build does not know, and the UI must neither break nor silently treat it
 * as Community. Gating is driven by `features`, never by this name.
 */
export type Edition = 'community' | 'personal' | 'pro' | (string & {})

/**
 * One window of the resource-history catalogue, as the engine declares it. The
 * whole catalogue is reported in every edition (it describes the product, not
 * the running tier), which is what lets the interface show a closed window and
 * name the edition that opens it without holding a table of its own.
 */
export interface HistoryWindowSpec {
  window: string
  seconds: number
  min_edition: Edition
}

export interface ResourceHistoryContract {
  /** The largest window the running edition opens, e.g. "6h". */
  max_window: string
  max_window_seconds: number
  windows: HistoryWindowSpec[]
}

export interface EditionResponse {
  edition: Edition
  organisation_name: string
  status_url?: string
  features: Record<string, boolean>
  /** capability -> minimum edition that opens it, projected from the backend registry */
  feature_editions?: Record<string, Edition>
  quotas?: Partial<Record<QuotaResource, QuotaEntry>>
  /** Absent on an engine older than the tiered history: no catalogue, no cap. */
  resource_history?: ResourceHistoryContract
}

/**
 * The structured part of an API error. `EDITION_REQUIRED` carries `feature`,
 * `QUOTA_EXCEEDED` and `HOST_LIMIT_REACHED` carry `resource` and `limit`, and
 * all three carry `required_edition`. Reading these is what replaces matching
 * on the message text.
 */
export interface ApiErrorDetail {
  code: string
  message: string
  feature?: string
  resource?: QuotaResource | string
  limit?: number
  required_edition?: Edition
  /** EDITION_REQUIRED on a history window: what was asked, and the current cap. */
  window?: string
  max_window?: string
}

export function fetchEdition(): Promise<EditionResponse> {
  return apiFetch(`${API_BASE}/edition`)
}

export interface LicenseStatus {
  status: string
  edition?: Edition
  plan: string
  message: string
  verified_at: string
  /** Empty for a perpetual license — never read an empty value as an expiry. */
  expires_at: string
  /**
   * Last day the license covers a newly released version. This is not an
   * expiry: a Personal license never expires, only its right to new versions is
   * bounded. Empty when there is no window, which is every Pro subscription.
   */
  updates_until: string
  /**
   * When a build released past `updates_until` loses the edition. Empty unless
   * the running build is outside the window.
   */
  update_grace_until: string
}

export function fetchLicenseStatus(): Promise<LicenseStatus> {
  return apiFetch(`${API_BASE}/license/status`)
}
