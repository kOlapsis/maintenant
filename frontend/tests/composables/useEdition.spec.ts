import { describe, it, expect, beforeEach, vi } from 'vitest'

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

// The classification used to live twice, in DefaultLayout and in EditionsPage,
// which is how a status ends up handled in one and falling through to a generic
// label in the other. It lives here now, so it is tested here.
describe('useEdition — license status classification', () => {
  beforeEach(() => {
    fetchEdition.mockReset()
    fetchLicenseStatus.mockReset()
  })

  async function loadLicense(status: string) {
    vi.resetModules()
    fetchEdition.mockResolvedValue({ edition: 'personal', organisation_name: '', features: {} })
    fetchLicenseStatus.mockResolvedValue({
      status,
      plan: 'personal',
      message: '',
      verified_at: '',
      expires_at: '',
      updates_until: '',
      update_grace_until: '',
    })
    const mod = await import('@/composables/useEdition')
    const handle = mod.useEdition()
    await handle.loadLicenseStatus()
    return handle
  }

  const cases: Array<{
    status: string
    label: string
    severity: 'warning' | 'critical'
    isIssue: boolean
  }> = [
    { status: '', label: 'LICENSE', severity: 'critical', isIssue: false },
    { status: 'active', label: 'LICENSE', severity: 'critical', isIssue: false },
    { status: 'inactive', label: 'LICENSE', severity: 'critical', isIssue: false },
    { status: 'grace', label: 'GRACE PERIOD', severity: 'warning', isIssue: true },
    { status: 'unreachable', label: 'LICENSE UNREACHABLE', severity: 'warning', isIssue: true },
    { status: 'expired', label: 'LICENSE EXPIRED', severity: 'critical', isIssue: true },
    { status: 'canceled', label: 'LICENSE CANCELED', severity: 'critical', isIssue: true },
    { status: 'revoked', label: 'LICENSE REVOKED', severity: 'critical', isIssue: true },
    { status: 'unknown', label: 'LICENSE INVALID', severity: 'critical', isIssue: true },
    { status: 'update_window_grace', label: 'UPDATES EXPIRED', severity: 'warning', isIssue: true },
    {
      status: 'update_window_ended',
      label: 'EDITION SUSPENDED',
      severity: 'critical',
      isIssue: true,
    },
    { status: 'something_new', label: 'LICENSE', severity: 'critical', isIssue: true },
  ]

  for (const c of cases) {
    it(`classifies ${c.status || '(empty)'}`, async () => {
      const e = await loadLicense(c.status)
      expect(e.licenseLabel.value).toBe(c.label)
      expect(e.licenseSeverity.value).toBe(c.severity)
      expect(e.hasLicenseIssue.value).toBe(c.isIssue)
    })
  }

  // A build past its window is an anomaly, so the Editions page must flag it
  // rather than treat it as the normal state of an unlicensed instance.
  it('treats both update-window statuses as faults', async () => {
    for (const status of ['update_window_grace', 'update_window_ended']) {
      const e = await loadLicense(status)
      expect(e.hasLicenseIssue.value).toBe(true)
    }
  })
})
