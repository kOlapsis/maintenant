<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts" generic="T extends string | number">
import { computed, useId } from 'vue'

export interface RadioOption<V extends string | number = string | number> {
  value: V
  label: string
  hint?: string
}

const props = withDefaults(
  defineProps<{
    options: RadioOption<T>[]
    name?: string
    ariaLabel?: string
    inline?: boolean
  }>(),
  { inline: false },
)

const model = defineModel<T | null>({ default: null })

const autoName = useId()
const groupName = computed(() => props.name ?? autoName)
</script>

<template>
  <div role="radiogroup" :aria-label="ariaLabel" :class="inline ? 'flex flex-wrap gap-4' : 'flex flex-col gap-2'">
    <label v-for="opt in options" :key="opt.value" class="inline-flex cursor-pointer items-start gap-2">
      <input
        v-model="model"
        type="radio"
        :name="groupName"
        :value="opt.value"
        class="mt-0.5 h-4 w-4 accent-[var(--mnt-accent)]"
      />
      <span>
        <span class="block text-sm text-mnt-secondary">{{ opt.label }}</span>
        <span v-if="opt.hint" class="block text-xs text-mnt-muted">{{ opt.hint }}</span>
      </span>
    </label>
  </div>
</template>
