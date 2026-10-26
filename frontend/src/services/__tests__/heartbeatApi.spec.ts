// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi } from 'vitest'
import { apiFetch } from '@/services/apiFetch'
import { rotatePingToken } from '@/services/heartbeatApi'

// 043-public-surfaces-hardening, FR-012, FR-013: the client calls the rotation route.

vi.mock('@/services/apiFetch', () => ({
  apiFetch: vi.fn(),
  apiFetchVoid: vi.fn(),
}))

describe('heartbeatApi.rotatePingToken', () => {
  it('posts to /api/v1/heartbeats/{id}/rotate-token and returns the heartbeat', async () => {
    const rotated = { id: '0199aaaa-0000-7000-8000-000000000001', ping_token: '9c2fcccc-0000-4000-8000-000000000003' }
    vi.mocked(apiFetch).mockResolvedValue(rotated)

    await expect(rotatePingToken(rotated.id)).resolves.toEqual(rotated)

    expect(apiFetch).toHaveBeenCalledTimes(1)
    const call = vi.mocked(apiFetch).mock.calls[0]
    expect(call).toBeDefined()
    const [url, init] = call!
    expect(String(url)).toMatch(/\/api\/v1\/heartbeats\/0199aaaa-0000-7000-8000-000000000001\/rotate-token$/)
    expect(init?.method).toBe('POST')
  })
})
