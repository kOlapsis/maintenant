// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { staleLabel, staleShort, staleTitle } from '../staleCause'

describe('staleCause', () => {
  it('names an unreachable runtime as such, the agent being connected', () => {
    const o = { stale: true, runtime_unreachable: true }
    expect(staleLabel(o)).toBe('Runtime unreachable')
    expect(staleShort(o)).toBe('unreachable')
    expect(staleTitle(o, 'running')).toBe('Runtime unreachable · last known: running')
  })

  it('names an offline agent otherwise', () => {
    const o = { stale: true, agent_offline: true }
    expect(staleLabel(o)).toBe('Agent offline')
    expect(staleShort(o)).toBe('offline')
    expect(staleTitle(o, 'healthy')).toBe('Agent offline · last known: healthy')
  })
})
