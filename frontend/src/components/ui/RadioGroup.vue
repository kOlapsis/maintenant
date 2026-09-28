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
import { computed, useId } from 'vue'

export interface RadioOption {
  value: string | number
  label: string
  hint?: string
}

const props = withDefaults(
  defineProps<{
    options: RadioOption[]
    name?: string
    ariaLabel?: string
    inline?: boolean
  }>(),
  { inline: false },
)

const model = defineModel<string | number | null>({ default: null })

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
