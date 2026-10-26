// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Acceptance tests of 047-audit-small-fixes, C13: an expired session leaves no API answer in the caches.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { reportAuthChallenge, sessionExpired, reauthStalled } from '../authGuard'

function fakeCaches(names: string[]) {
  const deleted: string[] = []
  const caches = {
    keys: vi.fn(async () => [...names]),
    delete: vi.fn(async (name: string) => {
      deleted.push(name)
      return true
    }),
  }
  vi.stubGlobal('caches', caches)
  return { caches, deleted }
}

/** Makes reportAuthChallenge take its "re-auth already failed" branch, which does not navigate (jsdom cannot). */
function stallReauth() {
  window.sessionStorage.setItem('mnt:reauth-at', String(Date.now()))
}

beforeEach(() => {
  sessionExpired.value = false
  reauthStalled.value = false
  window.sessionStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('reportAuthChallenge', () => {
  // FR-007, US3 scenario 2: every cache whose name starts with api- goes, nothing else does.
  it('drops every api-* cache and only those', async () => {
    const { deleted } = fakeCaches(['api-k1x2', 'static-k1x2', 'api-older', 'workbox-precache-v2'])
    stallReauth()

    reportAuthChallenge()

    await vi.waitFor(() => expect(deleted).toEqual(expect.arrayContaining(['api-k1x2', 'api-older'])))
    expect(deleted).toHaveLength(2)
    expect(reauthStalled.value).toBe(true)
  })

  it('purges on the branch that navigates back through the proxy too', async () => {
    const { deleted } = fakeCaches(['api-abc', 'static-abc'])

    reportAuthChallenge()

    await vi.waitFor(() => expect(deleted).toEqual(['api-abc']))
    expect(sessionExpired.value).toBe(true)
  })

  // Edge case "service worker sans cache api-*": nothing observable, no error.
  it('does nothing observable without an api-* cache, and never throws', async () => {
    const { caches, deleted } = fakeCaches(['static-abc', 'workbox-precache-v2'])
    stallReauth()

    expect(() => reportAuthChallenge()).not.toThrow()

    await vi.waitFor(() => expect(caches.keys).toHaveBeenCalled())
    expect(deleted).toEqual([])
  })
})
