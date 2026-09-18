// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'

const showToast = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useToast', () => ({ showToast }))

import { apiFetch, ApiError } from '../apiFetch'

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

beforeEach(() => {
  showToast.mockReset()
  vi.stubGlobal('fetch', vi.fn())
})

describe('ApiError.isDemoRefusal', () => {
  it('is true for a DEMO_MODE refusal', () => {
    expect(new ApiError(403, { code: 'DEMO_MODE', message: 'x' }, 'x').isDemoRefusal).toBe(true)
  })

  it('is false for any other refusal', () => {
    expect(
      new ApiError(403, { code: 'EDITION_REQUIRED', message: 'x' }, 'x').isDemoRefusal,
    ).toBe(false)
    expect(new ApiError(403, null, 'x').isDemoRefusal).toBe(false)
  })
})

describe('a DEMO_MODE refusal from the API', () => {
  const demoBody = {
    error: {
      code: 'DEMO_MODE',
      message: 'Demo mode: this instance is read-only, changes are disabled.',
    },
  }

  it('still throws, so the caller can revert its optimistic update', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse(403, demoBody))
    await expect(
      apiFetch('/api/v1/containers/x/restart', { method: 'POST' }),
    ).rejects.toBeInstanceOf(ApiError)
  })

  it('raises exactly one global toast', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse(403, demoBody))
    await expect(
      apiFetch('/api/v1/containers/x/restart', { method: 'POST' }),
    ).rejects.toThrow()
    expect(showToast).toHaveBeenCalledTimes(1)
    expect(showToast).toHaveBeenCalledWith(
      'This instance is read-only. Changes are disabled.',
      'warning',
      5000,
      { title: 'Demo mode', dedupeKey: 'demo-mode' },
    )
  })

  it('does not toast on an unrelated 403', async () => {
    vi.mocked(fetch).mockResolvedValue(
      jsonResponse(403, { error: { code: 'EDITION_REQUIRED', message: 'x' } }),
    )
    await expect(apiFetch('/api/v1/x')).rejects.toBeInstanceOf(ApiError)
    expect(showToast).not.toHaveBeenCalled()
  })
})
