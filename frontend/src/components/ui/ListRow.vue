<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { chipToneSolid, type ChipTone } from './listFilters'

const props = withDefaults(
  defineProps<{ tone?: ChipTone }>(),
  { tone: 'neutral' },
)

const emit = defineEmits<{ select: [] }>()

const toneColor = computed(() => chipToneSolid[props.tone])
</script>

<template>
  <div
    class="list-row flex items-center gap-3 px-4"
    :style="{ '--row-tone': toneColor }"
    role="button"
    tabindex="0"
    @click="emit('select')"
    @keydown.enter="emit('select')"
    @keydown.space.prevent="emit('select')"
  >
    <slot />
  </div>
</template>

<style scoped>
.list-row {
  position: relative;
  min-height: 56px;
  background: var(--mnt-bg-surface);
  border-bottom: 1px solid var(--mnt-border-subtle);
  cursor: pointer;
  transition: background-color 0.12s ease;
}
[data-density='compact'] .list-row {
  min-height: 44px;
}
.list-row:last-child {
  border-bottom: none;
}
.list-row:hover {
  background: var(--mnt-bg-hover);
}
.list-row:focus-visible {
  outline: 2px solid var(--mnt-accent);
  outline-offset: -2px;
}

/* The status gutter: a continuous band that reads as one spectrum down a long list. */
.list-row::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--row-tone);
}
</style>
