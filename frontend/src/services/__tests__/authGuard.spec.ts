// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { isAuthChallenge } from '../authGuard'

/** Builds a Response-like object; jsdom's Response cannot fake `redirected`. */
function response(init: {
  status?: number
  contentType?: string | null
  redirected?: boolean
  type?: string
}): Response {
  const { status = 200, contentType = 'application/json', redirected = false, type = 'basic' } = init
  return {
    status,
    ok: status >= 200 && status < 300,
    redirected,
    type,
    headers: { get: (name: string) => (name.toLowerCase() === 'content-type' ? contentType : null) },
  } as unknown as Response
}

describe('isAuthChallenge', () => {
  it('leaves genuine API answers alone', () => {
    expect(isAuthChallenge(response({ status: 200 }))).toBe(false)
    expect(isAuthChallenge(response({ status: 500 }))).toBe(false)
    expect(isAuthChallenge(response({ status: 204, contentType: null }))).toBe(false)
  })

  it('does not mistake the API 403s (PRO_REQUIRED, quotas) for an expired session', () => {
    expect(isAuthChallenge(response({ status: 403 }))).toBe(false)
    expect(isAuthChallenge(response({ status: 401 }))).toBe(false)
  })

  it('catches a proxy 302 that fetch followed to a login page', () => {
    expect(
      isAuthChallenge(response({ status: 200, contentType: 'text/html', redirected: true })),
    ).toBe(true)
  })

  it('catches a login page served straight up as 200 HTML', () => {
    expect(isAuthChallenge(response({ status: 200, contentType: 'text/html; charset=utf-8' }))).toBe(
      true,
    )
  })

  it('catches a non-JSON 401/403 from the proxy', () => {
    expect(isAuthChallenge(response({ status: 401, contentType: 'text/plain' }))).toBe(true)
    expect(isAuthChallenge(response({ status: 403, contentType: null }))).toBe(true)
  })

  it('catches the opaque redirect returned under redirect: manual', () => {
    expect(isAuthChallenge(response({ status: 0, contentType: null, type: 'opaqueredirect' }))).toBe(
      true,
    )
  })
})
