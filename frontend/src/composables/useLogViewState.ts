/*
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
*/

import { ref, watch, nextTick } from 'vue'
import type { UseLogStreamReturn } from './useLogStream'
import type { UseLogSearchReturn } from './useLogSearch'

export function useLogViewState(
  logStream: UseLogStreamReturn,
  search: UseLogSearchReturn,
) {
  const expandedJsonIds = ref(new Set<number>())

  function getActiveMatchOffset(lineIndex: number): number | null {
    const idx = search.currentMatchIndex.value
    if (idx < 0) return null
    const match = search.matches.value[idx]
    if (!match || match.lineIndex !== lineIndex) return null
    return match.startOffset
  }

  function toggleJsonExpand(lineId: number) {
    const s = new Set(expandedJsonIds.value)
    if (s.has(lineId)) {
      s.delete(lineId)
    } else {
      s.add(lineId)
    }
    expandedJsonIds.value = s
  }

  // Scroll current match into view
  watch(() => search.currentMatchIndex.value, () => {
    const idx = search.currentMatchIndex.value
    if (idx < 0) return
    const match = search.matches.value[idx]
    if (!match) return

    nextTick(() => {
      const container = logStream.scrollContainerRef.value
      if (!container) return
      const lineEl = container.querySelector(`[data-line-index="${match.lineIndex}"]`)
      if (lineEl) {
        lineEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      }
    })
  })

  // Clear expanded JSON when buffer trims
  watch(() => logStream.lines.value.length, (newLen, oldLen) => {
    if (newLen < oldLen) {
      expandedJsonIds.value = new Set()
    }
  })

  return {
    expandedJsonIds,
    getActiveMatchOffset,
    toggleJsonExpand,
  }
}