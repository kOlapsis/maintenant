// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { onMounted, onUnmounted } from 'vue'

// useVisibleInterval runs fn every intervalMs while the page is visible, skips
// every tick while it is hidden, and catches up once when it becomes visible
// again. Auto-registers on mount and cleans up on unmount.
export function useVisibleInterval(fn: () => void, intervalMs: number) {
  let timer: ReturnType<typeof setInterval> | null = null
  let missed = false

  function tick() {
    if (document.hidden) {
      missed = true
      return
    }
    fn()
  }

  function onVisibilityChange() {
    if (document.hidden || !missed) return
    missed = false
    fn()
  }

  onMounted(() => {
    timer = setInterval(tick, intervalMs)
    document.addEventListener('visibilitychange', onVisibilityChange)
  })

  onUnmounted(() => {
    if (timer) clearInterval(timer)
    timer = null
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })
}
