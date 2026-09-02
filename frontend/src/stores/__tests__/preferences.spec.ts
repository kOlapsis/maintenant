// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePreferencesStore } from '../preferences'

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('list view preference', () => {
  it('defaults to cards', () => {
    expect(usePreferencesStore().listView('containers')).toBe('cards')
  })

  it('keeps a separate view per page', () => {
    const prefs = usePreferencesStore()
    prefs.setListView('containers', 'table')
    prefs.setListView('endpoints', 'rows')
    expect(prefs.listView('containers')).toBe('table')
    expect(prefs.listView('endpoints')).toBe('rows')
    expect(prefs.listView('certificates')).toBe('cards')
  })

  it('persists the choice', () => {
    usePreferencesStore().setListView('heartbeats', 'rows')
    expect(localStorage.getItem('mnt-view:heartbeats')).toBe('rows')

    setActivePinia(createPinia())
    expect(usePreferencesStore().listView('heartbeats')).toBe('rows')
  })

  it('falls back to cards when the stored value is not a known view', () => {
    localStorage.setItem('mnt-view:containers', 'mosaic')
    expect(usePreferencesStore().listView('containers')).toBe('cards')
  })
})

describe('collapsible panels', () => {
  it('starts expanded and survives a reload once collapsed', () => {
    const prefs = usePreferencesStore()
    expect(prefs.isPanelCollapsed('top-consumers')).toBe(false)

    prefs.togglePanel('top-consumers')
    expect(prefs.isPanelCollapsed('top-consumers')).toBe(true)
    expect(localStorage.getItem('mnt-panel:top-consumers')).toBe('collapsed')

    setActivePinia(createPinia())
    expect(usePreferencesStore().isPanelCollapsed('top-consumers')).toBe(true)
  })

  it('tracks each panel independently', () => {
    const prefs = usePreferencesStore()
    prefs.togglePanel('top-consumers')
    expect(prefs.isPanelCollapsed('resource-charts')).toBe(false)
  })
})
