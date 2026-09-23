<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->
<script setup lang="ts" generic="T extends string">
import { ref } from 'vue'

export interface OptionCardItem<V extends string = string> {
  value: V
  label: string
  description?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    options: OptionCardItem<T>[]
    ariaLabel: string
    columns?: 2 | 3 | 4
  }>(),
  { columns: 2 },
)

const model = defineModel<T | null>({ default: null })

const cardRefs = ref<(HTMLButtonElement | null)[]>([])

function select(option: OptionCardItem<T>) {
  if (option.disabled) return
  model.value = option.value
}

function focusCard(index: number) {
  const lastIndex = props.options.length - 1
  const wrapped = index < 0 ? lastIndex : index > lastIndex ? 0 : index
  cardRefs.value[wrapped]?.focus()
}

function onKeydown(event: KeyboardEvent, index: number) {
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') focusCard(index + 1)
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') focusCard(index - 1)
  else if (event.key === 'Home') focusCard(0)
  else if (event.key === 'End') focusCard(props.options.length - 1)
  else return
  event.preventDefault()
}

const columnsClass: Record<2 | 3 | 4, string> = {
  2: 'sm:grid-cols-2',
  3: 'sm:grid-cols-3',
  4: 'sm:grid-cols-4',
}
</script>

<template>
  <div role="radiogroup" :aria-label="ariaLabel" class="grid grid-cols-1 gap-3" :class="columnsClass[columns]">
    <button
      v-for="(option, index) in options"
      :key="option.value"
      :ref="(el) => (cardRefs[index] = el as HTMLButtonElement | null)"
      type="button"
      role="radio"
      :aria-checked="model === option.value"
      :aria-disabled="option.disabled || undefined"
      :tabindex="model === option.value || (model === null && index === 0) ? 0 : -1"
      class="focus-ring relative flex flex-col items-center gap-2 rounded-lg border bg-mnt-elevated p-4 text-center transition-all"
      :class="[
        option.disabled ? 'cursor-not-allowed opacity-50' : '',
        model === option.value ? 'border-mnt-accent' : 'border-mnt-default hover:border-mnt-accent',
      ]"
      @click="select(option)"
      @keydown="onKeydown($event, index)"
    >
      <slot :option="option" :selected="model === option.value" />
    </button>
  </div>
</template>
