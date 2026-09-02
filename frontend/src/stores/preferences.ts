// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export type Density = 'compact' | 'comfortable'
export type MonitorView = 'grid' | 'list'
export type MonitorGroupBy = 'type' | 'severity'

/** How a monitor list renders its items: rich cards, dense rows, or a sortable table. */
export type ListView = 'cards' | 'rows' | 'table'
/**
 * Each list page remembers its own view, so Containers and Endpoints can differ.
 * 'demo' belongs to the design system page and never touches a real page's choice.
 */
export type ListScope = 'containers' | 'endpoints' | 'certificates' | 'heartbeats' | 'demo'

const LIST_VIEWS: readonly ListView[] = ['cards', 'rows', 'table']

export const usePreferencesStore = defineStore('preferences', () => {
  function getInitialDensity(): Density {
    // Fallback to the legacy "pb-" key so existing users keep their choice.
    const stored = localStorage.getItem('mnt-density') ?? localStorage.getItem('pb-density')
    if (stored === 'compact' || stored === 'comfortable') return stored
    return 'comfortable'
  }

  const density = ref<Density>(getInitialDensity())

  // Dashboard monitor view preferences (persisted like density).
  function getInitialView(): MonitorView {
    return (localStorage.getItem('mnt-monitors-view') ?? localStorage.getItem('pb-monitors-view')) === 'list' ? 'list' : 'grid'
  }
  function getInitialGroupBy(): MonitorGroupBy {
    return (localStorage.getItem('mnt-monitors-group') ?? localStorage.getItem('pb-monitors-group')) === 'severity' ? 'severity' : 'type'
  }

  const monitorsView = ref<MonitorView>(getInitialView())
  const monitorsGroupBy = ref<MonitorGroupBy>(getInitialGroupBy())

  watch(monitorsView, (v) => localStorage.setItem('mnt-monitors-view', v))
  watch(monitorsGroupBy, (v) => localStorage.setItem('mnt-monitors-group', v))

  function applyDensity(d: Density) {
    if (d === 'comfortable') {
      document.documentElement.removeAttribute('data-density')
    } else {
      document.documentElement.setAttribute('data-density', d)
    }
    localStorage.setItem('mnt-density', d)
  }

  function toggleDensity() {
    density.value = density.value === 'comfortable' ? 'compact' : 'comfortable'
  }

  watch(density, applyDensity, { immediate: true })

  // Per-page list view. Kept in one reactive record so a component only has to
  // name its scope, and written straight through to localStorage on change.
  const listViews = ref<Partial<Record<ListScope, ListView>>>({})

  function listView(scope: ListScope): ListView {
    const cached = listViews.value[scope]
    if (cached) return cached
    const stored = localStorage.getItem(`mnt-view:${scope}`) as ListView | null
    const resolved = stored && LIST_VIEWS.includes(stored) ? stored : 'cards'
    listViews.value[scope] = resolved
    return resolved
  }

  function setListView(scope: ListScope, view: ListView) {
    listViews.value[scope] = view
    localStorage.setItem(`mnt-view:${scope}`, view)
  }

  // Collapsed state for foldable page panels, keyed by panel name.
  const collapsedPanels = ref<Record<string, boolean>>({})

  function isPanelCollapsed(key: string): boolean {
    const cached = collapsedPanels.value[key]
    if (cached !== undefined) return cached
    const resolved = localStorage.getItem(`mnt-panel:${key}`) === 'collapsed'
    collapsedPanels.value[key] = resolved
    return resolved
  }

  function togglePanel(key: string) {
    const next = !isPanelCollapsed(key)
    collapsedPanels.value[key] = next
    localStorage.setItem(`mnt-panel:${key}`, next ? 'collapsed' : 'expanded')
  }

  return {
    density,
    toggleDensity,
    monitorsView,
    monitorsGroupBy,
    listView,
    setListView,
    isPanelCollapsed,
    togglePanel,
  }
})
