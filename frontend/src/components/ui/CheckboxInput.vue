<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { computed } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  label?: string
  hint?: string
  disabled?: boolean
  value?: string | number
}>()

const model = defineModel<boolean | (string | number)[]>({ default: false })

const isChecked = computed(() => {
  if (Array.isArray(model.value)) {
    return props.value !== undefined && model.value.includes(props.value)
  }
  return !!model.value
})

function onChange(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  if (Array.isArray(model.value)) {
    if (checked) {
      model.value =
        props.value !== undefined && !model.value.includes(props.value)
          ? [...model.value, props.value]
          : model.value
    } else {
      model.value = model.value.filter((v) => v !== props.value)
    }
  } else {
    model.value = checked
  }
}
</script>

<template>
  <div>
    <label class="inline-flex items-center gap-2" :class="disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'">
      <input
        v-bind="$attrs"
        type="checkbox"
        :checked="isChecked"
        :disabled="disabled"
        class="h-4 w-4 accent-[var(--mnt-accent)]"
        @change="onChange"
      />
      <span v-if="label || $slots.default" class="text-sm text-mnt-secondary">
        <slot>{{ label }}</slot>
      </span>
    </label>
    <p v-if="hint" class="mt-1 text-xs text-mnt-muted">{{ hint }}</p>
  </div>
</template>
