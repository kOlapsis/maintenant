<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import type { Component } from 'vue'

interface SegmentOption {
  value: string
  label?: string
  icon?: Component
  /** Accessible name when the option is icon-only. */
  title?: string
}

withDefaults(
  defineProps<{
    modelValue: string
    options: SegmentOption[]
    ariaLabel: string
  }>(),
  {},
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <div
    role="group"
    :aria-label="ariaLabel"
    class="inline-flex items-center gap-0.5 rounded-lg border border-pb-default bg-pb-surface p-0.5"
  >
    <button
      v-for="opt in options"
      :key="opt.value"
      type="button"
      :aria-pressed="modelValue === opt.value"
      :title="opt.title ?? opt.label"
      class="focus-ring inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold transition-colors"
      :class="
        modelValue === opt.value
          ? 'text-pb-accent'
          : 'text-pb-muted hover:text-pb-primary'
      "
      :style="modelValue === opt.value ? { backgroundColor: 'var(--pb-status-ok-bg)' } : undefined"
      @click="emit('update:modelValue', opt.value)"
    >
      <component :is="opt.icon" v-if="opt.icon" :size="14" aria-hidden="true" />
      <span v-if="opt.label">{{ opt.label }}</span>
    </button>
  </div>
</template>
