<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
/**
 * The status counts a page already showed, turned into the status filter.
 * One control instead of a read-only summary sitting above a redundant select.
 */
import { chipToneVars, type StatusChip } from './listFilters'

defineProps<{
  /** Selected status, or '' for no status filter. */
  modelValue: string
  chips: StatusChip[]
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const toneVars = chipToneVars

function toggle(value: string, current: string) {
  emit('update:modelValue', current === value ? '' : value)
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <button
      v-for="chip in chips"
      :key="chip.value"
      type="button"
      :aria-pressed="modelValue === chip.value"
      :disabled="chip.count === 0 && modelValue !== chip.value"
      class="status-chip focus-ring inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium transition"
      :class="{ 'is-active': modelValue === chip.value }"
      :style="{
        color: toneVars[chip.tone].fg,
        backgroundColor: toneVars[chip.tone].bg,
        borderColor: modelValue === chip.value ? toneVars[chip.tone].fg : 'transparent',
      }"
      @click="toggle(chip.value, modelValue)"
    >
      <span class="tabular-nums">{{ chip.count }}</span>
      <span>{{ chip.label }}</span>
    </button>
  </div>
</template>

<style scoped>
.status-chip {
  border: 1px solid transparent;
  cursor: pointer;
}
.status-chip:disabled {
  opacity: 0.45;
  cursor: default;
}
.status-chip:not(:disabled):hover {
  filter: brightness(1.08);
}
</style>
