// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Acceptance tests of 047-audit-small-fixes, C13: the app drops the api-* caches older service workers left behind.

import { describe, it, expect, vi, afterEach } from 'vitest'
import { initAuthGuard } from '../authGuard'

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

afterEach(() => {
  vi.unstubAllGlobals()
  window.history.replaceState(null, '', '/')
})

describe('initAuthGuard', () => {
  // FR-007, US3 scenario 2: every cache whose name starts with api- goes, nothing else does.
  it('drops every api-* cache and only those', async () => {
    const { deleted } = fakeCaches(['api-k1x2', 'static-k1x2', 'api-older', 'workbox-precache-v2'])

    initAuthGuard()

    await vi.waitFor(() => expect(deleted).toEqual(expect.arrayContaining(['api-k1x2', 'api-older'])))
    expect(deleted).toHaveLength(2)
  })

  it('purges on a boot that comes back from the proxy too', async () => {
    const { deleted } = fakeCaches(['api-abc', 'static-abc'])
    window.history.replaceState(null, '', '/dashboard?__reauth=k1x2')

    initAuthGuard()

    await vi.waitFor(() => expect(deleted).toEqual(['api-abc']))
  })

  // Edge case "service worker sans cache api-*": nothing observable, no error.
  it('does nothing observable without an api-* cache, and never throws', async () => {
    const { caches, deleted } = fakeCaches(['static-abc', 'workbox-precache-v2'])

    expect(() => initAuthGuard()).not.toThrow()

    await vi.waitFor(() => expect(caches.keys).toHaveBeenCalled())
    expect(deleted).toEqual([])
  })
})
