import { describe, it, expect } from 'vitest'
import { toDatetimeLocal } from '@/utils/time'

process.env.TZ = 'Europe/Paris'

describe('toDatetimeLocal', () => {
  it('renders winter time in the local zone', () => {
    expect(toDatetimeLocal('2026-12-01T10:00:00Z')).toBe('2026-12-01T11:00')
  })

  it('renders summer time in the local zone', () => {
    expect(toDatetimeLocal('2026-07-01T10:00:00Z')).toBe('2026-07-01T12:00')
  })

  it.each(['2026-12-01T10:00:00Z', '2026-07-01T10:00:00Z'])('round-trips %s', (iso) => {
    expect(new Date(toDatetimeLocal(iso)).toISOString()).toBe(new Date(iso).toISOString())
  })
})
