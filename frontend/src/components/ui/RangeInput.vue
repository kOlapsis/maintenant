<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->
<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    min: number
    max: number
    step?: number
  }>(),
  { step: 1 },
)

const model = defineModel<number>({ required: true })

// Chrome derives the unfilled range track from accent-color, which turns it
// near-black under our green; the track is drawn as a gradient instead.
const fillPercent = computed(() => {
  const ratio = (model.value - props.min) / (props.max - props.min)
  return `${Math.min(Math.max(ratio, 0), 1) * 100}%`
})
</script>

<template>
  <input
    v-model.number="model"
    type="range"
    :min="min"
    :max="max"
    :step="step"
    :aria-label="label"
    class="mnt-range w-full"
    :style="{ '--fill': fillPercent }"
  />
</template>

<style scoped>
.mnt-range {
  appearance: none;
  -webkit-appearance: none;
  height: 6px;
  border-radius: 999px;
  background: linear-gradient(to right, var(--mnt-accent) var(--fill), var(--mnt-bg-elevated) var(--fill));
  outline: none;
}

.mnt-range::-webkit-slider-thumb {
  appearance: none;
  -webkit-appearance: none;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--mnt-accent);
  cursor: pointer;
}

.mnt-range::-moz-range-thumb {
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 50%;
  background: var(--mnt-accent);
  cursor: pointer;
}
</style>
