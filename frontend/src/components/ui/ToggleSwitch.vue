<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    disabled?: boolean
    size?: 'md' | 'sm'
    showLabel?: boolean
  }>(),
  { disabled: false, size: 'md', showLabel: false },
)

const model = defineModel<boolean>({ default: false })

function toggle() {
  model.value = !model.value
}
</script>

<template>
  <span class="inline-flex items-center gap-2">
    <button
      type="button"
      role="switch"
      :aria-checked="model"
      :aria-label="label"
      :disabled="disabled"
      class="focus-ring relative inline-flex shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors disabled:cursor-not-allowed disabled:opacity-50"
      :class="[model ? 'bg-[var(--mnt-accent)]' : 'bg-mnt-elevated', size === 'md' ? 'h-6 w-11' : 'h-5 w-9']"
      @click.stop="toggle"
    >
      <span
        class="pointer-events-none inline-block transform rounded-full shadow-mnt-card transition-transform"
        :class="[
          model ? 'bg-[var(--mnt-text-inverted)]' : 'bg-mnt-primary',
          size === 'md' ? 'h-5 w-5' : 'h-4 w-4',
          model ? (size === 'md' ? 'translate-x-5' : 'translate-x-4') : 'translate-x-0',
        ]"
      />
    </button>
    <span v-if="showLabel" class="text-sm text-mnt-secondary">{{ label }}</span>
  </span>
</template>
