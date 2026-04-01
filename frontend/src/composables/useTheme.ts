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

import { ref, readonly, onMounted, onUnmounted } from 'vue'

export type ThemeValue = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'pb-theme'

const theme = ref<ThemeValue>('system')
const resolvedTheme = ref<ResolvedTheme>('dark')

function resolveOsTheme(): ResolvedTheme {
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

function applyTheme(resolved: ResolvedTheme) {
  document.documentElement.setAttribute('data-theme', resolved)
  resolvedTheme.value = resolved
}

function setTheme(value: ThemeValue) {
  theme.value = value
  localStorage.setItem(STORAGE_KEY, value)
  const resolved = value === 'system' ? resolveOsTheme() : value
  applyTheme(resolved)
}

let mediaQuery: MediaQueryList | null = null
let mediaListener: ((e: MediaQueryListEvent) => void) | null = null

function attachOsListener() {
  if (mediaQuery && mediaListener) return
  mediaQuery = window.matchMedia('(prefers-color-scheme: light)')
  mediaListener = (e: MediaQueryListEvent) => {
    if (theme.value === 'system') {
      applyTheme(e.matches ? 'light' : 'dark')
    }
  }
  mediaQuery.addEventListener('change', mediaListener)
}

function detachOsListener() {
  if (mediaQuery && mediaListener) {
    mediaQuery.removeEventListener('change', mediaListener)
    mediaQuery = null
    mediaListener = null
  }
}

export function useTheme() {
  onMounted(() => {
    const stored = localStorage.getItem(STORAGE_KEY) as ThemeValue | null
    const valid: ThemeValue[] = ['light', 'dark', 'system']
    theme.value = stored && valid.includes(stored) ? stored : 'system'

    const resolved = theme.value === 'system' ? resolveOsTheme() : theme.value
    applyTheme(resolved)
    attachOsListener()
  })

  onUnmounted(() => {
    detachOsListener()
  })

  return {
    theme: readonly(theme),
    resolvedTheme: readonly(resolvedTheme),
    setTheme,
  }
}
