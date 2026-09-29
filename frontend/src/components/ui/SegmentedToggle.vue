<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->
<script setup lang="ts" generic="T extends string">
import type { Component } from 'vue'

interface SegmentOption<V extends string = string> {
  value: V
  label?: string
  icon?: Component
  /** Accessible name when the option is icon-only. */
  title?: string
  disabled?: boolean
  locked?: boolean
  dataTest?: string
}

withDefaults(
  defineProps<{
    modelValue: T
    options: SegmentOption<T>[]
    ariaLabel: string
  }>(),
  {},
)

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()
</script>

<template>
  <div
    role="group"
    :aria-label="ariaLabel"
    class="inline-flex items-center gap-0.5 rounded-lg border border-mnt-default bg-mnt-surface p-0.5"
  >
    <button
      v-for="opt in options"
      :key="opt.value"
      type="button"
      :data-test="opt.dataTest"
      :data-locked="opt.locked ? 'true' : undefined"
      :disabled="opt.disabled"
      :aria-pressed="modelValue === opt.value"
      :title="opt.title ?? opt.label"
      class="focus-ring inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50"
      :class="
        modelValue === opt.value
          ? 'text-mnt-accent'
          : 'text-mnt-muted hover:text-mnt-primary'
      "
      :style="modelValue === opt.value ? { backgroundColor: 'var(--mnt-status-ok-bg)' } : undefined"
      @click="emit('update:modelValue', opt.value)"
    >
      <component :is="opt.icon" v-if="opt.icon" :size="14" aria-hidden="true" />
      <span v-if="opt.label">{{ opt.label }}</span>
    </button>
  </div>
</template>
