/*
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
*/

import { ref, watch, type Ref } from 'vue'
import type { LogLine } from './useLogStream'

export interface SearchMatch {
  lineIndex: number
  startOffset: number
  endOffset: number
}

export interface UseLogSearchReturn {
  query: Ref<string>
  isOpen: Ref<boolean>
  isRegex: Ref<boolean>
  isCaseSensitive: Ref<boolean>
  isValid: Ref<boolean>
  matches: Ref<SearchMatch[]>
  currentMatchIndex: Ref<number>
  open: () => void
  close: () => void
  setQuery: (q: string) => void
  nextMatch: () => void
  prevMatch: () => void
  toggleRegex: () => void
  toggleCaseSensitive: () => void
  getLineMatches: (lineIndex: number) => SearchMatch[]
}

const DEBOUNCE_MS = 150

export function useLogSearch(lines: Ref<LogLine[]>): UseLogSearchReturn {
  const query = ref('')
  const isOpen = ref(false)
  const isRegex = ref(false)
  const isCaseSensitive = ref(false)
  const isValid = ref(true)
  const matches = ref<SearchMatch[]>([])
  const currentMatchIndex = ref(-1)

  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  function computeMatches() {
    const q = query.value
    if (!q) {
      matches.value = []
      currentMatchIndex.value = -1
      isValid.value = true
      return
    }

    const newMatches: SearchMatch[] = []

    if (isRegex.value) {
      let re: RegExp
      try {
        const flags = isCaseSensitive.value ? 'g' : 'gi'
        re = new RegExp(q, flags)
        isValid.value = true
      } catch {
        isValid.value = false
        matches.value = []
        currentMatchIndex.value = -1
        return
      }

      for (let i = 0; i < lines.value.length; i++) {
        const line = lines.value[i]!
        re.lastIndex = 0
        let m: RegExpExecArray | null
        while ((m = re.exec(line.raw)) !== null) {
          if (m[0].length === 0) {
            re.lastIndex++
            continue
          }
          newMatches.push({
            lineIndex: i,
            startOffset: m.index,
            endOffset: m.index + m[0].length,
          })
        }
      }
    } else {
      isValid.value = true
      const searchTerm = isCaseSensitive.value ? q : q.toLowerCase()

      for (let i = 0; i < lines.value.length; i++) {
        const line = lines.value[i]!
        const haystack = isCaseSensitive.value ? line.raw : line.raw.toLowerCase()
        let pos = 0
        while (pos < haystack.length) {
          const idx = haystack.indexOf(searchTerm, pos)
          if (idx === -1) break
          newMatches.push({
            lineIndex: i,
            startOffset: idx,
            endOffset: idx + searchTerm.length,
          })
          pos = idx + 1
        }
      }
    }

    matches.value = newMatches

    if (newMatches.length === 0) {
      currentMatchIndex.value = -1
    } else if (currentMatchIndex.value >= newMatches.length) {
      currentMatchIndex.value = 0
    } else if (currentMatchIndex.value < 0) {
      currentMatchIndex.value = 0
    }
  }

  function debouncedCompute() {
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(computeMatches, DEBOUNCE_MS)
  }

  function open() {
    isOpen.value = true
  }

  function close() {
    isOpen.value = false
    query.value = ''
    matches.value = []
    currentMatchIndex.value = -1
    isValid.value = true
  }

  function setQuery(q: string) {
    query.value = q
    debouncedCompute()
  }

  function nextMatch() {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
      computeMatches()
    }
    if (matches.value.length === 0) return
    currentMatchIndex.value = (currentMatchIndex.value + 1) % matches.value.length
  }

  function prevMatch() {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
      computeMatches()
    }
    if (matches.value.length === 0) return
    currentMatchIndex.value =
      (currentMatchIndex.value - 1 + matches.value.length) % matches.value.length
  }

  function toggleRegex() {
    isRegex.value = !isRegex.value
    computeMatches()
  }

  function toggleCaseSensitive() {
    isCaseSensitive.value = !isCaseSensitive.value
    computeMatches()
  }

  function getLineMatches(lineIndex: number): SearchMatch[] {
    return matches.value.filter(m => m.lineIndex === lineIndex)
  }

  watch(() => lines.value.length, () => {
    if (query.value) computeMatches()
  })

  return {
    query,
    isOpen,
    isRegex,
    isCaseSensitive,
    isValid,
    matches,
    currentMatchIndex,
    open,
    close,
    setQuery,
    nextMatch,
    prevMatch,
    toggleRegex,
    toggleCaseSensitive,
    getLineMatches,
  }
}
