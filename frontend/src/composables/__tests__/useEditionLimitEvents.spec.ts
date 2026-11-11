// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'

// 051-community-label-limits, contract "Les deux nouveaux événements rejoignent la
// liste des événements qui rafraîchissent les quotas", FR-017.

const listened = vi.hoisted(() => [] as string[])

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: (name: string) => listened.push(name), off: vi.fn() },
}))
vi.mock('@/services/editionApi', () => ({
  fetchEdition: vi.fn(() => new Promise(() => {})),
  fetchLicenseStatus: vi.fn(async () => null),
}))

import '@/composables/useEdition'

describe('useEdition: quota refresh', () => {
  it('reloads the quotas when an endpoint or a certificate changes state', () => {
    expect(listened).toContain('endpoint.limit_changed')
    expect(listened).toContain('certificate.limit_changed')
  })
})
