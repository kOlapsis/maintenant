// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

/**
 * Shared fetch wrapper for all API services.
 */

import { AuthChallengeError, isAuthChallenge, probeAuth, reportAuthChallenge } from './authGuard'
import type { ApiErrorDetail } from './editionApi'
import { showToast } from '@/composables/useToast'

/**
 * An API error that keeps the structured body instead of flattening it to a
 * message. Edition and quota refusals carry the capability, the resource, the
 * limit and the edition that lifts it — callers read those fields rather than
 * matching on English text, which breaks the moment the wording changes.
 */
export class ApiError extends Error {
  readonly status: number
  readonly detail: ApiErrorDetail | null

  constructor(status: number, detail: ApiErrorDetail | null, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.detail = detail
  }

  get code(): string {
    return this.detail?.code ?? ''
  }

  /** True for the refusals that mean "your edition does not allow this". */
  get isEditionRefusal(): boolean {
    return this.code === 'EDITION_REQUIRED'
  }

  /** True for the refusal a demo instance sends back to every mutation. */
  get isDemoRefusal(): boolean {
    return this.code === 'DEMO_MODE'
  }

  /** True for the refusals that mean "you have reached a cap". */
  get isQuotaRefusal(): boolean {
    return this.code === 'QUOTA_EXCEEDED' || this.code === 'HOST_LIMIT_REACHED'
  }

  /**
   * True when the database is momentarily unreachable. The request failed for
   * a reason that repairs itself, so callers should keep what they already
   * have on screen rather than clearing it: the storage banner explains why
   * the data is not refreshing (FR-023).
   */
  get isStorageOutage(): boolean {
    return this.status === 503 && this.code === 'STORAGE_UNAVAILABLE'
  }
}

export async function toApiError(res: Response): Promise<ApiError> {
  const body = await res.json().catch(() => ({}))
  const detail = (body?.error ?? null) as ApiErrorDetail | null
  const err = new ApiError(res.status, detail, detail?.message || `HTTP ${res.status}`)
  if (err.isDemoRefusal) {
    showToast('This instance is read-only. Changes are disabled.', 'warning', 5000, {
      title: 'Demo mode',
      dedupeKey: 'demo-mode',
    })
  }
  return err
}

// Sentinel agent id the backend sends for server-local entities. Treat it as
// "local" everywhere a host is grouped or displayed.
export const LOCAL_AGENT = '00000000-0000-0000-0000-000000000000'

export function isLocalAgent(id?: string | null): boolean {
  return !id || id === LOCAL_AGENT
}

/**
 * fetch, but an auth-proxy challenge becomes an AuthChallengeError instead of a
 * login page the caller would try to parse as JSON.
 */
export async function guardedFetch(url: string, init?: RequestInit): Promise<Response> {
  let res: Response
  try {
    res = await fetch(url, init)
  } catch (err) {
    if (await probeAuth()) throw new AuthChallengeError()
    throw err
  }

  if (isAuthChallenge(res)) {
    reportAuthChallenge()
    throw new AuthChallengeError()
  }

  return res
}

export async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await guardedFetch(url, init)

  if (!res.ok) {
    throw await toApiError(res)
  }

  return res.json()
}

export async function apiFetchVoid(url: string, init?: RequestInit): Promise<void> {
  const res = await guardedFetch(url, init)

  if (!res.ok) {
    throw await toApiError(res)
  }
}
