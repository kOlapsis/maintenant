<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useVirtualizer, type VirtualItem } from '@tanstack/vue-virtual'
import SeverityRow from './SeverityRow.vue'
import type { GridItem } from './statusGrid'

// Windowed list for large groups (> threshold). A bounded scroll container keeps
// a 300+ item group fluid without affecting the page-scroll layout of small ones.
const props = defineProps<{ items: GridItem[]; maxHeight?: number }>()
const emit = defineEmits<{ select: [item: GridItem] }>()

const parentRef = ref<HTMLElement | null>(null)

const rowVirtualizer = useVirtualizer(
  computed(() => ({
    count: props.items.length,
    getScrollElement: () => parentRef.value,
    estimateSize: () => 44,
    overscan: 8,
  })),
)

const totalSize = computed(() => rowVirtualizer.value.getTotalSize())
const rows = computed(() =>
  rowVirtualizer.value
    .getVirtualItems()
    .map((vrow) => ({ vrow, item: props.items[vrow.index] }))
    .filter((r): r is { vrow: VirtualItem; item: GridItem } => r.item !== undefined),
)
</script>

<template>
  <div ref="parentRef" class="overflow-auto" :style="{ maxHeight: `${maxHeight ?? 420}px` }">
    <div :style="{ height: `${totalSize}px`, position: 'relative', width: '100%' }">
      <div
        v-for="row in rows"
        :key="row.vrow.index"
        :style="{
          position: 'absolute',
          top: '0',
          left: '0',
          width: '100%',
          height: `${row.vrow.size}px`,
          transform: `translateY(${row.vrow.start}px)`,
        }"
      >
        <SeverityRow
          :severity="row.item.severity"
          :name="row.item.name"
          :kind="row.item.kind"
          :description="row.item.description"
          :host="row.item.host"
          :metric="row.item.meta"
          @select="emit('select', row.item)"
        />
      </div>
    </div>
  </div>
</template>
