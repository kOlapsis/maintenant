import { describe, it, expect, vi, beforeEach } from 'vitest'

const { fetchEdition } = vi.hoisted(() => ({ fetchEdition: vi.fn() }))

vi.mock('@/services/sseBus', () => ({ sseBus: { on: vi.fn(), off: vi.fn() } }))
vi.mock('@/services/editionApi', () => ({ fetchEdition, fetchLicenseStatus: vi.fn() }))
vi.mock('@/services/authGuard', () => ({ onProbePayload: vi.fn() }))

const pro = { edition: 'pro', organisation_name: 'Acme', features: { smtp: true } }

async function freshEdition() {
  vi.resetModules()
  const { useEdition } = await import('@/composables/useEdition')
  return useEdition()
}

describe('useEdition load failures', () => {
  beforeEach(() => {
    fetchEdition.mockReset()
  })

  it('keeps the known edition when a reload fails', async () => {
    fetchEdition.mockResolvedValueOnce(pro)
    const ed = await freshEdition()
    await vi.waitFor(() => expect(ed.editionName.value).toBe('pro'))

    fetchEdition.mockRejectedValue(new Error('HTTP 503'))
    await ed.reload()

    expect(ed.editionName.value).toBe('pro')
    expect(ed.hasFeature('smtp')).toBe(true)
  })

  it('falls back to community when no edition was ever loaded', async () => {
    fetchEdition.mockRejectedValue(new Error('HTTP 503'))
    const ed = await freshEdition()
    await ed.reload()
    expect(ed.editionName.value).toBe('community')
  })
})
