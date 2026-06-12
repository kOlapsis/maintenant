// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import {
  severityFromStatus,
  severityFromAlert,
  severityRank,
  severityVar,
  isActionable,
} from '../useSeverity'

describe('useSeverity', () => {
  it('maps UnifiedStatus → severity (presentation-only)', () => {
    expect(severityFromStatus('ok')).toBe('ok')
    expect(severityFromStatus('warning')).toBe('warning')
    expect(severityFromStatus('down')).toBe('incident')
    expect(severityFromStatus('paused')).toBe('neutral')
    expect(severityFromStatus('unknown')).toBe('unknown')
  })

  it('degrades any unknown status string to unknown', () => {
    expect(severityFromStatus('garbage')).toBe('unknown')
  })

  it('maps AlertSeverity → severity', () => {
    expect(severityFromAlert('critical')).toBe('incident')
    expect(severityFromAlert('warning')).toBe('warning')
    expect(severityFromAlert('info')).toBe('neutral')
  })

  it('ranks problems first, mirroring the store statusOrder', () => {
    expect(severityRank('incident')).toBeLessThan(severityRank('warning'))
    expect(severityRank('warning')).toBeLessThan(severityRank('unknown'))
    expect(severityRank('unknown')).toBeLessThan(severityRank('ok'))
    expect(severityRank('ok')).toBeLessThan(severityRank('neutral'))
  })

  it('builds tokenised CSS var references, never a hex', () => {
    expect(severityVar('incident')).toBe('var(--pb-sev-incident)')
    expect(severityVar('ok', 'bg')).toBe('var(--pb-sev-ok-bg)')
    expect(severityVar('warning', 'text')).toBe('var(--pb-sev-warning-text)')
    expect(severityVar('unknown', 'border')).toBe('var(--pb-sev-unknown-border)')
  })

  it('flags only incident and warning as actionable', () => {
    expect(isActionable('incident')).toBe(true)
    expect(isActionable('warning')).toBe(true)
    expect(isActionable('ok')).toBe(false)
    expect(isActionable('neutral')).toBe(false)
    expect(isActionable('unknown')).toBe(false)
  })
})
