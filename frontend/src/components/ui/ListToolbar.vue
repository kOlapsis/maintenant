<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { SlidersHorizontal } from 'lucide-vue-next'
import SearchInput from './SearchInput.vue'
import StatusFilterChips from './StatusFilterChips.vue'
import type { StatusChip } from './listFilters'
import ViewModeToggle from './ViewModeToggle.vue'
import type { ListScope } from '@/stores/preferences'

withDefaults(
  defineProps<{
    scope: ListScope
    search: string
    status: string
    chips: StatusChip[]
    /** Items left after every filter, shown next to the query. */
    resultCount?: number
    searchPlaceholder?: string
    /** Secondary filters currently narrowing the list, shown on the Filters button. */
    activeFilterCount?: number
  }>(),
  { resultCount: undefined, searchPlaceholder: 'Search', activeFilterCount: 0 },
)

const emit = defineEmits<{
  'update:search': [value: string]
  'update:status': [value: string]
  reset: []
}>()

const menuOpen = ref(false)
const menuRoot = ref<HTMLElement | null>(null)

function onDocumentPointerDown(e: PointerEvent) {
  if (!menuRoot.value?.contains(e.target as Node)) menuOpen.value = false
}

watch(menuOpen, (open) => {
  if (open) document.addEventListener('pointerdown', onDocumentPointerDown)
  else document.removeEventListener('pointerdown', onDocumentPointerDown)
})

onUnmounted(() => document.removeEventListener('pointerdown', onDocumentPointerDown))
</script>

<template>
  <div class="list-toolbar mb-4 flex flex-col gap-3 py-3">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
      <SearchInput
        :model-value="search"
        :placeholder="searchPlaceholder"
        :result-count="resultCount"
        class="lg:max-w-xs lg:flex-1"
        @update:model-value="emit('update:search', $event)"
      />

      <div class="flex items-center gap-2 lg:ml-auto">
        <div v-if="$slots.filters" ref="menuRoot" class="relative">
          <button
            type="button"
            class="focus-ring inline-flex min-h-[38px] items-center gap-1.5 rounded-lg border border-mnt-default bg-mnt-surface px-3 text-xs font-semibold text-mnt-secondary"
            :aria-expanded="menuOpen"
            aria-haspopup="dialog"
            @click="menuOpen = !menuOpen"
          >
            <SlidersHorizontal :size="14" aria-hidden="true" />
            Filters
            <span
              v-if="activeFilterCount > 0"
              class="rounded-full px-1.5 text-[10px] font-bold tabular-nums"
              :style="{ backgroundColor: 'var(--mnt-accent)', color: 'var(--mnt-text-inverted)' }"
            >{{ activeFilterCount }}</span>
          </button>

          <div
            v-if="menuOpen"
            class="filter-menu absolute right-0 z-30 mt-2 w-72 rounded-xl border border-mnt-default bg-mnt-surface p-3"
            role="dialog"
            aria-label="Filters"
            @keydown.esc="menuOpen = false"
          >
            <div class="flex flex-col gap-3">
              <slot name="filters" />
            </div>
            <button
              type="button"
              class="focus-ring mt-3 w-full rounded-lg border border-mnt-default py-1.5 text-xs font-semibold text-mnt-muted hover:text-mnt-primary"
              @click="emit('reset'); menuOpen = false"
            >
              Clear all filters
            </button>
          </div>
        </div>

        <ViewModeToggle :scope="scope" />
      </div>
    </div>

    <StatusFilterChips
      v-if="chips.length > 0"
      :model-value="status"
      :chips="chips"
      @update:model-value="emit('update:status', $event)"
    />
  </div>
</template>

<style scoped>
.list-toolbar {
  position: sticky;
  top: 0;
  z-index: 20;
  background: var(--mnt-bg-primary);
}
.filter-menu {
  box-shadow: var(--mnt-shadow-elevated);
}
</style>
