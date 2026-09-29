<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts" generic="T extends string | number">
export interface SelectOption<V extends string | number = string | number> {
  value: V
  label: string
  disabled?: boolean
}

defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    options?: SelectOption<T>[]
    invalid?: boolean
    size?: 'md' | 'sm'
  }>(),
  { options: () => [], invalid: false, size: 'md' },
)

const model = defineModel<T | null>({ default: null })
</script>

<template>
  <select
    v-bind="$attrs"
    v-model="model"
    :aria-invalid="invalid || undefined"
    class="focus-ring w-full rounded-lg border bg-mnt-elevated text-mnt-primary disabled:cursor-not-allowed disabled:opacity-60"
    :class="[
      invalid ? 'border-[var(--mnt-status-down-text)]' : 'border-mnt-default',
      size === 'md' ? 'min-h-[44px] px-3 text-sm' : 'px-3 py-1.5 text-xs',
    ]"
  >
    <option v-for="opt in options" :key="opt.value" :value="opt.value" :disabled="opt.disabled">
      {{ opt.label }}
    </option>
    <slot />
  </select>
</template>
