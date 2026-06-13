<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { LayoutGrid, List, ChevronDown } from 'lucide-vue-next'
import StatusTile from './StatusTile.vue'
import SeverityRow from './SeverityRow.vue'
import SegmentedToggle from './SegmentedToggle.vue'
import VirtualRows from './VirtualRows.vue'
import { severityVar } from '@/composables/useSeverity'
import type { GridItem, GridGroup } from './statusGrid'

const props = withDefaults(
  defineProps<{
    groups: GridGroup[]
    view?: 'grid' | 'list'
    virtualizeThreshold?: number
  }>(),
  { view: 'grid', virtualizeThreshold: 150 },
)

const emit = defineEmits<{
  'update:view': [value: 'grid' | 'list']
  select: [item: GridItem]
  hover: [item: GridItem | null]
}>()

// Collapse is local UI state. Backfill new groups as data loads, preserving any
// manual toggles the user already made.
const collapsed = ref<Set<string>>(new Set())
const seen = ref<Set<string>>(new Set())
watch(
  () => props.groups,
  (groups) => {
    const next = new Set(collapsed.value)
    for (const g of groups) {
      if (!seen.value.has(g.key)) {
        seen.value.add(g.key)
        if (g.collapsedByDefault) next.add(g.key)
      }
    }
    collapsed.value = next
  },
  { immediate: true },
)

function toggle(key: string) {
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsed.value = next
}
const isCollapsed = (key: string) => collapsed.value.has(key)

function counts(g: GridGroup) {
  let incident = 0
  let warning = 0
  let ok = 0
  for (const it of g.items) {
    if (it.severity === 'incident') incident++
    else if (it.severity === 'warning') warning++
    else if (it.severity === 'ok') ok++
  }
  return { incident, warning, ok }
}

const viewOptions = [
  { value: 'grid', label: 'Grid', icon: LayoutGrid },
  { value: 'list', label: 'List', icon: List },
]
const viewModel = computed<string>({
  get: () => props.view,
  set: (v) => emit('update:view', v === 'list' ? 'list' : 'grid'),
})
</script>

<template>
  <div>
    <div class="mb-3 flex items-center gap-3">
      <slot name="bar" />
      <div class="ml-auto">
        <SegmentedToggle v-model="viewModel" :options="viewOptions" ariaLabel="Monitor view" />
      </div>
    </div>

    <div class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface">
      <div
        v-for="g in groups"
        :key="g.key"
        class="border-t border-mnt-subtle first:border-t-0"
      >
        <button
          type="button"
          class="focus-ring flex w-full items-center gap-2.5 px-4 py-2.5 text-left transition-colors hover:bg-mnt-elevated"
          :aria-expanded="!isCollapsed(g.key)"
          @click="toggle(g.key)"
        >
          <ChevronDown
            :size="14"
            class="text-mnt-muted transition-transform"
            :class="isCollapsed(g.key) ? '-rotate-90' : ''"
            aria-hidden="true"
          />
          <component :is="g.icon" v-if="g.icon" :size="14" class="text-mnt-muted" aria-hidden="true" />
          <span class="text-xs font-semibold text-mnt-primary">{{ g.label }}</span>
          <span class="font-mono text-[11px] text-mnt-muted">{{ g.items.length }}</span>
          <span class="ml-auto flex items-center gap-2.5 font-mono text-[11px] text-mnt-muted">
            <span v-if="counts(g).incident" class="flex items-center gap-1">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: severityVar('incident') }" />
              {{ counts(g).incident }}
            </span>
            <span v-if="counts(g).warning" class="flex items-center gap-1">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: severityVar('warning') }" />
              {{ counts(g).warning }}
            </span>
            <span class="flex items-center gap-1">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: severityVar('ok') }" />
              {{ counts(g).ok }}
            </span>
          </span>
        </button>

        <div v-show="!isCollapsed(g.key)" class="px-4 mnt-4 pt-1">
          <!-- Large groups: windowed list (always rows, even in grid view). -->
          <VirtualRows
            v-if="g.items.length > virtualizeThreshold"
            :items="g.items"
            @select="emit('select', $event)"
          />
          <!-- Grid view -->
          <div
            v-else-if="view === 'grid'"
            class="grid gap-1.5"
            style="grid-template-columns: repeat(auto-fill, minmax(118px, 1fr))"
          >
            <StatusTile
              v-for="it in g.items"
              :key="it.id"
              :severity="it.severity"
              :name="it.name"
              :meta="it.meta"
              @select="emit('select', it)"
              @mouseenter="emit('hover', it)"
              @mouseleave="emit('hover', null)"
            />
          </div>
          <!-- List view -->
          <div v-else class="flex flex-col">
            <SeverityRow
              v-for="it in g.items"
              :key="it.id"
              :severity="it.severity"
              :name="it.name"
              :kind="it.kind"
              :description="it.description"
              :metric="it.meta"
              @select="emit('select', it)"
            />
          </div>
        </div>
      </div>

      <div v-if="groups.length === 0">
        <slot name="empty" />
      </div>
    </div>
  </div>
</template>
