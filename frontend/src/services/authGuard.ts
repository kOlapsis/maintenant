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
 * Session handling for deployments sitting behind an authentication proxy
 * (Authentik, oauth2-proxy, Cloudflare Access…).
 *
 * The service worker serves the app shell from cache, so the SPA boots even
 * when the proxy session is gone. Every API call then lands on a login page
 * instead of the API. This module recognises that answer and sends the browser
 * back through the proxy so it can challenge the user again.
 */

import { ref } from 'vue'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

/** Query param carried by the re-auth navigation; the SW denylist skips it. */
export const REAUTH_PARAM = '__reauth'
const REAUTH_STAMP_KEY = 'mnt:reauth-at'
/** A challenge landing again within this window means the re-auth did not stick. */
const REAUTH_LOOP_WINDOW_MS = 15_000
const PROBE_THROTTLE_MS = 5_000

/** true once a proxy challenge has been seen; the overlay takes over the UI. */
export const sessionExpired = ref(false)
/** true when the automatic round-trip already failed: only a manual retry is left. */
export const reauthStalled = ref(false)

export class AuthChallengeError extends Error {
  constructor() {
    super('Session expired')
    this.name = 'AuthChallengeError'
  }
}

/**
 * Tells an auth-proxy answer apart from a genuine API answer.
 *
 * The API always replies JSON — including its own 403s (EDITION_REQUIRED, quotas),
 * which must never be mistaken for an expired session.
 */
export function isAuthChallenge(res: Response): boolean {
  // redirect: 'manual' turns the proxy's 302 into an opaque redirect.
  if (res.type === 'opaqueredirect') return true

  // A 401 always comes from the proxy: the API has no authentication of its
  // own and never sends one. Checked before the content type because
  // oauth2-proxy answers `Accept: application/json` calls — the session probe
  // among them — with a JSON 401.
  if (res.status === 401) return true

  const contentType = res.headers.get('content-type') ?? ''
  if (contentType.includes('json')) return false

  if (res.status === 403) return true
  // fetch followed the proxy's 302 and handed us its login page under a 200.
  if (res.redirected) return true
  return res.ok && contentType.includes('html')
}

function lastReauthAt(): number {
  try {
    return Number(window.sessionStorage.getItem(REAUTH_STAMP_KEY)) || 0
  } catch {
    return 0
  }
}

function stampReauth(): void {
  try {
    window.sessionStorage.setItem(REAUTH_STAMP_KEY, String(Date.now()))
  } catch {
    // Private mode without storage: we lose the loop guard, not the re-auth.
  }
}

/**
 * Navigates through the proxy so it can challenge us. The `__reauth` param is
 * denylisted in the service worker, so this navigation reaches the network
 * instead of being answered from the precached app shell.
 */
export function startReauth(): void {
  stampReauth()
  const url = new URL(window.location.href)
  url.searchParams.set(REAUTH_PARAM, Date.now().toString(36))
  window.location.assign(url.toString())
}

/** Called on every detected challenge; triggers the overlay and one re-auth. */
export function reportAuthChallenge(): void {
  if (sessionExpired.value) return
  sessionExpired.value = true

  if (Date.now() - lastReauthAt() < REAUTH_LOOP_WINDOW_MS) {
    // We just came back from the proxy and it is challenging us again: stop
    // bouncing and let the user decide.
    reauthStalled.value = true
    return
  }
  startReauth()
}

let probeInFlight: Promise<boolean> | null = null
let lastProbeAt = 0

/**
 * The session probe calls /edition, so its response body is the edition payload.
 * Rather than throwing it away, hand it to whoever tracks the edition — that is
 * one fewer request, and it realigns capabilities after a license transition.
 *
 * A callback rather than a direct import: useEdition already imports the API
 * layer, which imports this module, so importing it back would be a cycle.
 */
let probePayloadSink: ((payload: unknown) => void) | null = null

export function onProbePayload(fn: (payload: unknown) => void): void {
  probePayloadSink = fn
}

/**
 * Distinguishes an expired session from a real network outage when fetch
 * rejects outright — a proxy hosted on another origin fails the request on
 * CORS instead of answering it.
 */
export async function probeAuth(): Promise<boolean> {
  if (sessionExpired.value) return true
  if (probeInFlight) return probeInFlight
  if (Date.now() - lastProbeAt < PROBE_THROTTLE_MS) return false
  lastProbeAt = Date.now()

  probeInFlight = (async () => {
    try {
      const res = await fetch(`${API_BASE}/edition?__probe=1`, {
        redirect: 'manual',
        cache: 'no-store',
        headers: { Accept: 'application/json' },
      })
      if (isAuthChallenge(res)) {
        reportAuthChallenge()
        return true
      }
      if (res.ok && probePayloadSink) {
        const payload = await res.json().catch(() => null)
        if (payload) probePayloadSink(payload)
      }
    } catch {
      // Nothing answered at all: the network is down, not the session.
    }
    return false
  })()

  try {
    return await probeInFlight
  } finally {
    probeInFlight = null
  }
}

/** Strips the re-auth param from the address bar once the app has booted. */
export function initAuthGuard(): void {
  const url = new URL(window.location.href)
  if (!url.searchParams.has(REAUTH_PARAM)) return
  url.searchParams.delete(REAUTH_PARAM)
  window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`)
}
