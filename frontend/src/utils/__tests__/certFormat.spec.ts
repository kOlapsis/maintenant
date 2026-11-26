// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { certDaysRemaining, formatDaysRemaining } from '../certFormat'
import type { CertCheckResult, CertMonitor } from '@/services/certificateApi'

const check = (over: Partial<CertCheckResult>) => over as CertCheckResult
const cert = (over: Partial<CertMonitor>) => over as CertMonitor

describe('certDaysRemaining', () => {
  it('reads the days the list sends on the certificate', () => {
    expect(certDaysRemaining(cert({ days_remaining: 42, latest_check: check({ not_after: '2027-01-01T00:00:00Z' }) }))).toBe(42)
  })

  it('reads the days of the latest check', () => {
    expect(certDaysRemaining(cert({ latest_check: check({ not_after: '2027-01-01T00:00:00Z', days_remaining: 12 }) }))).toBe(12)
  })

  it('is unknown when the check never read an expiry date', () => {
    expect(certDaysRemaining(cert({ latest_check: check({ days_remaining: -1, error_message: 'dial tcp: timeout' }) }))).toBeNull()
    expect(certDaysRemaining(cert({ days_remaining: -1 }))).toBeNull()
    expect(certDaysRemaining(cert({}))).toBeNull()
  })

  it('keeps a certificate that really expired', () => {
    expect(certDaysRemaining(cert({ latest_check: check({ not_after: '2020-01-01T00:00:00Z', days_remaining: -1 }) }))).toBe(-1)
  })
})

describe('formatDaysRemaining', () => {
  it('never calls an unknown value expired', () => {
    expect(formatDaysRemaining(null)).toBe('Unknown')
    expect(formatDaysRemaining(undefined)).toBe('Unknown')
    expect(formatDaysRemaining(-3)).toBe('Expired')
    expect(formatDaysRemaining(0)).toBe('Today')
    expect(formatDaysRemaining(9)).toBe('9d')
  })
})
