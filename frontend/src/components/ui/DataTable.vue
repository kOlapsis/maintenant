<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts" generic="T">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ChevronDown, ChevronUp } from 'lucide-vue-next'
import { chipToneSolid, type ChipTone } from './listFilters'

export interface Column {
  key: string
  label: string
  sortable?: boolean
  align?: 'left' | 'right'
  /** Any CSS width; the column grid is built from these. */
  width?: string
  /** Drop the column on narrow screens rather than squeezing every column. */
  priority?: 'always' | 'sm' | 'md' | 'lg'
}

const props = withDefaults(
  defineProps<{
    columns: Column[]
    rows: T[]
    rowKey: (row: T) => string
    /** Sort key for a column; return a number or a string. */
    sortValue: (row: T, key: string) => string | number | undefined
    /** Colours the status gutter on the left edge of each row. */
    tone?: (row: T) => ChipTone
    defaultSort?: string
    caption: string
  }>(),
  { tone: undefined, defaultSort: undefined },
)

const emit = defineEmits<{ select: [row: T] }>()

const sortKey = ref<string | undefined>(props.defaultSort)
const sortDir = ref<'asc' | 'desc'>('asc')

// Columns are dropped in JS rather than hidden in CSS: a display:none cell in a
// grid still holds its track, which would leave a gap where the column was.
const viewportWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const onResize = () => { viewportWidth.value = window.innerWidth }
onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))

const minWidths: Record<NonNullable<Column['priority']>, number> = {
  always: 0,
  sm: 640,
  md: 768,
  lg: 1024,
}

const visibleColumns = computed(() =>
  props.columns.filter((c) => viewportWidth.value >= minWidths[c.priority ?? 'always']),
)

const gridTemplate = computed(() =>
  visibleColumns.value.map((c) => c.width ?? 'minmax(0, 1fr)').join(' '),
)

const sortedRows = computed(() => {
  const key = sortKey.value
  if (!key) return props.rows
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...props.rows].sort((a, b) => {
    const av = props.sortValue(a, key)
    const bv = props.sortValue(b, key)
    if (av === undefined && bv === undefined) return 0
    if (av === undefined) return 1
    if (bv === undefined) return -1
    if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir
    return String(av).localeCompare(String(bv), undefined, { numeric: true }) * dir
  })
})

function toggleSort(col: Column) {
  if (!col.sortable) return
  if (sortKey.value === col.key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = col.key
    sortDir.value = 'asc'
  }
}

function ariaSort(col: Column): 'ascending' | 'descending' | 'none' | undefined {
  if (!col.sortable) return undefined
  if (sortKey.value !== col.key) return 'none'
  return sortDir.value === 'asc' ? 'ascending' : 'descending'
}
</script>

<template>
  <div class="data-table overflow-x-auto rounded-xl border border-mnt-default bg-mnt-surface">
    <table class="w-full">
      <caption class="sr-only">{{ caption }}</caption>
      <thead>
        <tr class="head-row" :style="{ gridTemplateColumns: gridTemplate }">
          <th
            v-for="col in visibleColumns"
            :key="col.key"
            scope="col"
            :aria-sort="ariaSort(col)"
            :class="col.align === 'right' ? 'text-right' : 'text-left'"
          >
            <button
              v-if="col.sortable"
              type="button"
              class="focus-ring inline-flex items-center gap-1 hover:text-mnt-primary"
              @click="toggleSort(col)"
            >
              {{ col.label }}
              <component
                :is="sortDir === 'asc' ? ChevronUp : ChevronDown"
                v-if="sortKey === col.key"
                :size="12"
                aria-hidden="true"
              />
            </button>
            <span v-else>{{ col.label }}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="row in sortedRows"
          :key="rowKey(row)"
          class="body-row"
          :style="{
            gridTemplateColumns: gridTemplate,
            '--row-tone': tone ? chipToneSolid[tone(row)] : 'transparent',
          }"
          tabindex="0"
          @click="emit('select', row)"
          @keydown.enter="emit('select', row)"
          @keydown.space.prevent="emit('select', row)"
        >
          <td
            v-for="col in visibleColumns"
            :key="col.key"
            :class="col.align === 'right' ? 'text-right' : 'text-left'"
          >
            <slot :name="`cell-${col.key}`" :row="row" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
/*
 * Grid rows rather than table-layout: the columns stay aligned while each cell
 * is free to hold a badge, a bar, or a sparkline.
 */
.data-table table {
  display: block;
  min-width: 100%;
}
.data-table thead,
.data-table tbody {
  display: block;
}
.head-row,
.body-row {
  display: grid;
  align-items: center;
  gap: 0.75rem;
  padding: 0 0.875rem;
}
.head-row {
  height: 34px;
  border-bottom: 1px solid var(--mnt-border-default);
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--mnt-text-muted);
}
.body-row {
  position: relative;
  height: 40px;
  border-bottom: 1px solid var(--mnt-border-subtle);
  font-size: 0.8125rem;
  color: var(--mnt-text-secondary);
  cursor: pointer;
}
[data-density='compact'] .body-row {
  height: 32px;
}
.body-row:last-child {
  border-bottom: none;
}
.body-row:hover {
  background: var(--mnt-bg-hover);
}
.body-row:focus-visible {
  outline: 2px solid var(--mnt-accent);
  outline-offset: -2px;
}

/* The status gutter: one continuous band down the left edge of the list. */
.body-row::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--row-tone);
}

.head-row th,
.body-row td {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
