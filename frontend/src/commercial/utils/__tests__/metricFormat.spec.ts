// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect } from 'vitest'
import { formatBytes, formatMetricValue } from '@/commercial/utils/metricFormat'

describe('formatBytes', () => {
  it('scales to the right unit', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(120_890_709)).toBe('115.3 MB')
    expect(formatBytes(5 * 1024 ** 3)).toBe('5.0 GB')
  })

  it('does not blow past the largest unit', () => {
    expect(formatBytes(1024 ** 6)).toContain('TB')
  })

  it('handles non-finite input', () => {
    expect(formatBytes(NaN)).toBe('—')
    expect(formatBytes(Infinity)).toBe('—')
  })
})

describe('formatMetricValue', () => {
  it('renders each metric in its own unit', () => {
    expect(formatMetricValue('cpu', 42.35)).toBe('42.4%')
    expect(formatMetricValue('memory', 120_890_709)).toBe('115.3 MB')
    expect(formatMetricValue('swap', 1024)).toBe('1.0 KB')
    expect(formatMetricValue('disk_space', 1024 ** 3)).toBe('1.0 GB')
    expect(formatMetricValue('network_io', 65_536)).toBe('64.0 KB/s')
    expect(formatMetricValue('disk_io', 65_536)).toBe('64.0 KB/s')
    expect(formatMetricValue('load', 1.234)).toBe('1.23')
  })

  it('falls back to a plain number rather than guessing a unit', () => {
    expect(formatMetricValue('something_new', 7.25)).toBe('7.3')
  })
})
