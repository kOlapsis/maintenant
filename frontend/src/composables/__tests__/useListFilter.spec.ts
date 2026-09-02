// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { computed, ref } from 'vue'
import { useListFilter } from '../useListFilter'

interface Monitor {
  name: string
  target: string
  status: string
  type: string
  note?: string
}

const monitors: Monitor[] = [
  { name: 'traefik', target: 'https://maintenant.dev/health', status: 'up', type: 'http' },
  { name: 'shm-app', target: 'https://metrics.kolapsis.com/hc', status: 'down', type: 'http' },
  { name: 'registry', target: 'registry.kolapsis.com:5000', status: 'up', type: 'tcp' },
  { name: 'authentik', target: 'https://auth.kolapsis.com/live', status: 'degraded', type: 'http' },
]

function setup(extra?: Parameters<typeof useListFilter<Monitor>>[1]['extra']) {
  const source = ref(monitors)
  return useListFilter(source, {
    searchFields: (m) => [m.name, m.target, m.note],
    status: (m) => m.status,
    extra,
  })
}

describe('useListFilter', () => {
  it('returns everything when nothing is set', () => {
    expect(setup().filtered.value).toHaveLength(4)
  })

  it('matches any search field, case-insensitively', () => {
    const f = setup()
    f.search.value = 'KOLAPSIS.COM'
    expect(f.filtered.value.map((m) => m.name)).toEqual(['shm-app', 'registry', 'authentik'])
  })

  it('ignores surrounding whitespace in the query', () => {
    const f = setup()
    f.search.value = '  traefik  '
    expect(f.filtered.value.map((m) => m.name)).toEqual(['traefik'])
  })

  it('skips undefined search fields instead of throwing', () => {
    const f = setup()
    f.search.value = 'note'
    expect(f.filtered.value).toHaveLength(0)
  })

  it('filters on status', () => {
    const f = setup()
    f.status.value = 'up'
    expect(f.filtered.value.map((m) => m.name)).toEqual(['traefik', 'registry'])
  })

  it('combines search, status and secondary predicates', () => {
    const typeFilter = ref('')
    const f = setup({
      type: computed(() =>
        typeFilter.value ? (m: Monitor) => m.type === typeFilter.value : null,
      ),
    })
    f.status.value = 'up'
    typeFilter.value = 'tcp'
    expect(f.filtered.value.map((m) => m.name)).toEqual(['registry'])
  })

  it('counts only the secondary filters that are active', () => {
    const typeFilter = ref('')
    const f = setup({
      type: computed(() =>
        typeFilter.value ? (m: Monitor) => m.type === typeFilter.value : null,
      ),
    })
    expect(f.activeFilterCount.value).toBe(0)
    typeFilter.value = 'http'
    expect(f.activeFilterCount.value).toBe(1)
  })

  it('counts each status against the other filters, not the whole list', () => {
    const f = setup()
    expect(f.statusCounts.value.get('up')).toBe(2)

    // kolapsis.com leaves shm-app (down), registry (up) and authentik (degraded).
    f.search.value = 'kolapsis.com'
    expect(f.statusCounts.value.get('up')).toBe(1)
    expect(f.statusCounts.value.get('down')).toBe(1)
  })

  it('a status count matches the rows that status leaves standing', () => {
    const f = setup()
    f.search.value = 'kolapsis.com'
    const promised = f.statusCounts.value.get('down') ?? 0
    f.status.value = 'down'
    expect(f.filtered.value).toHaveLength(promised)
  })

  it('the selected status does not shrink its own count', () => {
    const f = setup()
    f.status.value = 'up'
    expect(f.statusCounts.value.get('degraded')).toBe(1)
  })

  it('reset clears search and status', () => {
    const f = setup()
    f.search.value = 'traefik'
    f.status.value = 'up'
    f.reset()
    expect(f.search.value).toBe('')
    expect(f.status.value).toBe('')
    expect(f.filtered.value).toHaveLength(4)
  })
})
