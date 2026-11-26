import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

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

  afterEach(() => {
    vi.useRealTimers()
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

  it('stays unknown, never Community, while no edition could be loaded', async () => {
    fetchEdition.mockRejectedValue(new Error('HTTP 503'))
    const ed = await freshEdition()
    await ed.reload()

    expect(ed.editionName.value).toBeNull()
    expect(ed.isCommunity.value).toBe(false)
    expect(ed.isLoaded.value).toBe(false)
  })

  it('retries with a growing delay until the edition loads', async () => {
    vi.useFakeTimers()
    fetchEdition
      .mockRejectedValueOnce(new Error('HTTP 503'))
      .mockRejectedValueOnce(new Error('HTTP 503'))
      .mockResolvedValue(pro)
    const ed = await freshEdition()
    let ready = false
    void ed.whenLoaded().then(() => (ready = true))
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchEdition).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1_000)
    expect(fetchEdition).toHaveBeenCalledTimes(2)
    expect(ready).toBe(false)

    await vi.advanceTimersByTimeAsync(1_999)
    expect(fetchEdition).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)

    expect(fetchEdition).toHaveBeenCalledTimes(3)
    expect(ed.editionName.value).toBe('pro')
    expect(ready).toBe(true)
  })
})
