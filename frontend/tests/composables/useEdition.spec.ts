import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref } from 'vue'

// useEdition holds module-level state and kicks off a fetch on import, so each
// case resets the module registry and stubs the API before importing it.
const fetchEdition = vi.fn()
const fetchLicenseStatus = vi.fn()

vi.mock('@/services/editionApi', () => ({
  fetchEdition: (...a: unknown[]) => fetchEdition(...a),
  fetchLicenseStatus: (...a: unknown[]) => fetchLicenseStatus(...a),
}))
vi.mock('@/services/sseBus', () => ({ sseBus: { on: vi.fn(), off: vi.fn() } }))

type EditionPayload = {
  edition: string
  features?: Record<string, boolean>
  feature_editions?: Record<string, string>
}

async function loadEdition(payload: EditionPayload) {
  vi.resetModules()
  fetchEdition.mockResolvedValue({
    organisation_name: '',
    features: {},
    ...payload,
  })
  fetchLicenseStatus.mockResolvedValue(null)
  const mod = await import('@/composables/useEdition')
  const handle = mod.useEdition()
  await handle.reload()
  return handle
}

describe('useEdition — the three editions', () => {
  beforeEach(() => {
    fetchEdition.mockReset()
    fetchLicenseStatus.mockReset()
  })

  it('community: not paid, ranks 0', async () => {
    const e = await loadEdition({ edition: 'community' })
    expect(e.editionName.value).toBe('community')
    expect(e.editionRank.value).toBe(0)
    expect(e.isCommunity.value).toBe(true)
    expect(e.isPersonal.value).toBe(false)
    expect(e.isPro.value).toBe(false)
    expect(e.isPaid.value).toBe(false)
  })

  it('personal: paid, and never classified as community', async () => {
    const e = await loadEdition({ edition: 'personal' })
    expect(e.editionName.value).toBe('personal')
    expect(e.editionRank.value).toBe(1)
    expect(e.isPersonal.value).toBe(true)
    expect(e.isCommunity.value).toBe(false)
    expect(e.isPro.value).toBe(false)
    expect(e.isPaid.value).toBe(true)
  })

  it('pro: paid, ranks 2', async () => {
    const e = await loadEdition({ edition: 'pro' })
    expect(e.editionRank.value).toBe(2)
    expect(e.isPro.value).toBe(true)
    expect(e.isCommunity.value).toBe(false)
    expect(e.isPaid.value).toBe(true)
  })

  // FR-045: an engine ahead of the UI must not break it, and must not have its
  // edition silently downgraded to Community — that would canvass someone who
  // has paid, and would be the unsafe direction.
  it('an unknown edition is treated as paid, with a null rank', async () => {
    const e = await loadEdition({ edition: 'enterprise' })
    expect(e.editionName.value).toBe('enterprise')
    expect(e.editionRank.value).toBeNull()
    expect(e.isCommunity.value).toBe(false)
    expect(e.isPaid.value).toBe(true)
    expect(e.isPro.value).toBe(false)
    expect(e.isPersonal.value).toBe(false)
  })
})

describe('useEdition — requiredEditionFor', () => {
  beforeEach(() => {
    fetchEdition.mockReset()
    fetchLicenseStatus.mockReset()
  })

  it('reads the minimum edition from the backend registry, one per tier', async () => {
    const e = await loadEdition({
      edition: 'personal',
      feature_editions: {
        swarm_dashboard: 'community',
        incidents: 'personal',
        alert_escalation: 'pro',
      },
    })
    expect(e.requiredEditionFor('swarm_dashboard')).toBe('community')
    expect(e.requiredEditionFor('incidents')).toBe('personal')
    expect(e.requiredEditionFor('alert_escalation')).toBe('pro')
  })

  // The matrix is never duplicated in TypeScript, so an unknown capability
  // yields null and the caller falls back to a generic message rather than
  // guessing "pro" and telling the user to buy the wrong thing.
  it('returns null for a capability the engine did not declare', async () => {
    const e = await loadEdition({ edition: 'pro', feature_editions: { incidents: 'personal' } })
    expect(e.requiredEditionFor('not_a_capability')).toBeNull()
  })

  it('returns null when the engine sends no registry at all', async () => {
    const e = await loadEdition({ edition: 'community' })
    expect(e.requiredEditionFor('incidents')).toBeNull()
  })
})

describe('useEdition — gating is driven by features, not by the edition name', () => {
  beforeEach(() => {
    fetchEdition.mockReset()
    fetchLicenseStatus.mockReset()
  })

  it('hasFeature follows the flags even when the edition name is unknown', async () => {
    const e = await loadEdition({
      edition: 'enterprise',
      features: { incidents: true, alert_escalation: false },
    })
    expect(e.hasFeature('incidents')).toBe(true)
    expect(e.hasFeature('alert_escalation')).toBe(false)
    expect(e.hasFeature('absent_from_the_payload')).toBe(false)
  })
})
