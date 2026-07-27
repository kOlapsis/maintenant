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

/**
 * Shared fetch wrapper for all API services.
 */

import { AuthChallengeError, isAuthChallenge, probeAuth, reportAuthChallenge } from './authGuard'

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
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message || `HTTP ${res.status}`)
  }

  return res.json()
}

export async function apiFetchVoid(url: string, init?: RequestInit): Promise<void> {
  const res = await guardedFetch(url, init)

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message || `HTTP ${res.status}`)
  }
}
