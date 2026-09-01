// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { UnifiedMonitor } from '@/stores/dashboard'

const state = vi.hoisted(() => ({
  dashboard: { monitors: [] as unknown[], searchQuery: '' },
  prefs: { monitorsGroupBy: 'type' as string },
}))

vi.mock('@/stores/dashboard', () => ({ useDashboardStore: () => state.dashboard }))
vi.mock('@/stores/preferences', () => ({ usePreferencesStore: () => state.prefs }))

import { useMonitorGroups } from '../useMonitorGroups'

function monitor(name: string, host: string | null): UnifiedMonitor {
  return {
    id: `container:${name}`,
    type: 'container',
    name,
    status: 'ok',
    statusLabel: 'Running',
    subtitle: 'traefik:v3',
    group: null,
    sparklineData: null,
    sparklineType: null,
    metricValue: null,
    metricLabel: null,
    host,
    link: { name: 'containers' },
    updatedAt: '2026-08-31T00:00:00Z',
  }
}

beforeEach(() => {
  state.dashboard.searchQuery = ''
  state.prefs.monitorsGroupBy = 'type'
})

describe('useMonitorGroups — host attribution', () => {
  // Issue #94: a dozen identically-named containers are indistinguishable
  // without knowing which host reported each one.
  it('carries the host down to the grid item', () => {
    state.dashboard.monitors = [monitor('traefik', 'edge-1.lan'), monitor('traefik-2', 'Local')]
    const items = useMonitorGroups().monitorGroups.value[0]!.items
    expect(items.map((i) => i.host)).toEqual(['edge-1.lan', 'Local'])
  })

  it('leaves the host unset when the store hides it', () => {
    state.dashboard.monitors = [monitor('traefik', null)]
    const items = useMonitorGroups().monitorGroups.value[0]!.items
    expect(items[0]!.host).toBeUndefined()
  })
})
