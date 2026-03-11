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
  const matchesByLine = ref(new Map<number, SearchMatch[]>())
  const currentMatchIndex = ref(-1)

  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  let lastScannedLength = 0

  function buildMatchesByLine(allMatches: SearchMatch[]): Map<number, SearchMatch[]> {
    const map = new Map<number, SearchMatch[]>()
    for (const m of allMatches) {
      let arr = map.get(m.lineIndex)
      if (!arr) {
        arr = []
        map.set(m.lineIndex, arr)
      }
      arr.push(m)
    }
    return map
  }

  function searchLine(lineIndex: number, line: LogLine, re: RegExp | null, searchTerm: string): SearchMatch[] {
    const result: SearchMatch[] = []
    if (re) {
      re.lastIndex = 0
      let m: RegExpExecArray | null
      while ((m = re.exec(line.text)) !== null) {
        if (m[0].length === 0) {
          re.lastIndex++
          continue
        }
        result.push({ lineIndex, startOffset: m.index, endOffset: m.index + m[0].length })
      }
    } else {
      const haystack = isCaseSensitive.value ? line.text : line.text.toLowerCase()
      let pos = 0
      while (pos < haystack.length) {
        const idx = haystack.indexOf(searchTerm, pos)
        if (idx === -1) break
        result.push({ lineIndex, startOffset: idx, endOffset: idx + searchTerm.length })
        pos = idx + 1
      }
    }
    return result
  }

  function buildSearchParams(): { re: RegExp | null; searchTerm: string } | null {
    const q = query.value
    if (!q) return null

    if (isRegex.value) {
      try {
        const flags = isCaseSensitive.value ? 'g' : 'gi'
        return { re: new RegExp(q, flags), searchTerm: '' }
      } catch {
        isValid.value = false
        return null
      }
    }
    isValid.value = true
    return { re: null, searchTerm: isCaseSensitive.value ? q : q.toLowerCase() }
  }

  function computeMatches() {
    const q = query.value
    if (!q) {
      matches.value = []
      matchesByLine.value = new Map()
      currentMatchIndex.value = -1
      isValid.value = true
      lastScannedLength = lines.value.length
      return
    }

    isValid.value = true
    const params = buildSearchParams()
    if (!params) {
      matches.value = []
      matchesByLine.value = new Map()
      currentMatchIndex.value = -1
      lastScannedLength = lines.value.length
      return
    }

    const newMatches: SearchMatch[] = []
    for (let i = 0; i < lines.value.length; i++) {
      const lineMatches = searchLine(i, lines.value[i]!, params.re, params.searchTerm)
      newMatches.push(...lineMatches)
    }

    matches.value = newMatches
    matchesByLine.value = buildMatchesByLine(newMatches)
    lastScannedLength = lines.value.length

    if (newMatches.length === 0) {
      currentMatchIndex.value = -1
    } else if (currentMatchIndex.value >= newMatches.length) {
      currentMatchIndex.value = 0
    } else if (currentMatchIndex.value < 0) {
      currentMatchIndex.value = 0
    }
  }

  function scanNewLines() {
    if (!query.value) return

    const params = buildSearchParams()
    if (!params) return

    const startIdx = lastScannedLength
    const allLines = lines.value
    if (startIdx >= allLines.length) return

    const newMatches: SearchMatch[] = []
    for (let i = startIdx; i < allLines.length; i++) {
      const lineMatches = searchLine(i, allLines[i]!, params.re, params.searchTerm)
      newMatches.push(...lineMatches)
    }

    if (newMatches.length > 0) {
      matches.value = [...matches.value, ...newMatches]
      const map = new Map(matchesByLine.value)
      for (const m of newMatches) {
        let arr = map.get(m.lineIndex)
        if (!arr) {
          arr = []
          map.set(m.lineIndex, arr)
        }
        arr.push(m)
      }
      matchesByLine.value = map

      if (currentMatchIndex.value < 0) {
        currentMatchIndex.value = 0
      }
    }
    lastScannedLength = allLines.length
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
    matchesByLine.value = new Map()
    currentMatchIndex.value = -1
    isValid.value = true
    lastScannedLength = 0
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
    return matchesByLine.value.get(lineIndex) ?? []
  }

  watch(() => lines.value.length, (newLen, oldLen) => {
    if (!query.value) return
    if (newLen < oldLen) {
      // Buffer trimmed — full recompute needed (line indices shifted)
      lastScannedLength = 0
      computeMatches()
    } else if (newLen > oldLen) {
      scanNewLines()
    }
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
