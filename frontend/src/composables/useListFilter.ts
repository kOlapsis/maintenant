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

import { computed, ref, type ComputedRef, type Ref } from 'vue'

export interface ListFilterOptions<T> {
  /** Text a search query is matched against, per item. Undefined entries are skipped. */
  searchFields: (item: T) => (string | undefined | null)[]
  /** The item's status, compared against the selected status chip. */
  status: (item: T) => string
  /**
   * Secondary predicates, keyed by filter name. A predicate returning true for
   * every item counts as inactive and is not reported in `activeFilterCount`.
   */
  extra?: Record<string, ComputedRef<((item: T) => boolean) | null>>
}

/**
 * Search and status filtering, shared by every monitor list. Filtering happens
 * on the rows the page already holds: no round trip, results land as you type.
 */
export function useListFilter<T>(
  source: Ref<T[]> | ComputedRef<T[]>,
  options: ListFilterOptions<T>,
) {
  const search = ref('')
  const status = ref('')

  const query = computed(() => search.value.trim().toLowerCase())

  /**
   * Everything the other filters keep, before the status chips narrow it down.
   * The chip counts are read from here so a chip promising "3 down" really does
   * leave 3 rows standing once it is clicked.
   */
  const beforeStatus = computed(() => {
    const q = query.value
    const extras = Object.values(options.extra ?? {})
      .map((e) => e.value)
      .filter((p): p is (item: T) => boolean => p !== null)

    return source.value.filter((item) => {
      for (const predicate of extras) {
        if (!predicate(item)) return false
      }
      if (!q) return true
      return options.searchFields(item).some((f) => f?.toLowerCase().includes(q))
    })
  })

  const filtered = computed(() => {
    const s = status.value
    if (!s) return beforeStatus.value
    return beforeStatus.value.filter((item) => options.status(item) === s)
  })

  /** How many items each status value would leave, for the chip counts. */
  const statusCounts = computed(() => {
    const counts = new Map<string, number>()
    for (const item of beforeStatus.value) {
      const s = options.status(item)
      counts.set(s, (counts.get(s) ?? 0) + 1)
    }
    return counts
  })

  /** Secondary filters currently narrowing the list, for the Filters badge. */
  const activeFilterCount = computed(
    () => Object.values(options.extra ?? {}).filter((e) => e.value !== null).length,
  )

  function reset() {
    search.value = ''
    status.value = ''
  }

  return { search, status, query, beforeStatus, filtered, statusCounts, activeFilterCount, reset }
}
