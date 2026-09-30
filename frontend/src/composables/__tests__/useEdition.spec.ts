// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'

const listened = vi.hoisted(() => new Set<string>())

vi.mock('@/services/sseBus', () => ({
  sseBus: { on: (name: string) => listened.add(name), off: vi.fn() },
}))
vi.mock('@/services/editionApi', () => ({
  fetchEdition: vi.fn().mockResolvedValue({ edition: 'community', organisation_name: '', features: {} }),
  fetchLicenseStatus: vi.fn(),
}))
vi.mock('@/services/authGuard', () => ({ onProbePayload: vi.fn() }))

await import('@/composables/useEdition')

describe('useEdition quota refresh', () => {
  it('reloads the edition when a status component is created or deleted', () => {
    expect(listened).toContain('status.component_created')
    expect(listened).toContain('status.component_deleted')
    expect(listened).not.toContain('status.component_changed')
  })
})
