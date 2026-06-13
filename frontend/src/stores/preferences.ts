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

  return {
    density,
    toggleDensity,
    monitorsView,
    monitorsGroupBy,
  }
})
