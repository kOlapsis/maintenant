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
defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    type?: 'text' | 'email' | 'password' | 'number' | 'url' | 'datetime-local' | 'time'
    invalid?: boolean
    mono?: boolean
    size?: 'md' | 'sm'
  }>(),
  { type: 'text', invalid: false, mono: false, size: 'md' },
)

const model = defineModel<string | number | null>({ default: '' })

function onInput(event: Event) {
  const raw = (event.target as HTMLInputElement).value
  model.value = props.type === 'number' ? (raw === '' ? null : Number(raw)) : raw
}
</script>

<template>
  <input
    v-bind="$attrs"
    :type="type"
    :value="model ?? ''"
    :aria-invalid="invalid || undefined"
    class="focus-ring w-full rounded-lg border bg-mnt-elevated text-mnt-primary placeholder:text-mnt-muted disabled:cursor-not-allowed disabled:opacity-60"
    :class="[
      invalid ? 'border-[var(--mnt-status-down-text)]' : 'border-mnt-default',
      mono ? 'font-mono' : '',
      size === 'md' ? 'min-h-[44px] px-3 text-sm' : 'px-3 py-1.5 text-xs',
    ]"
    @input="onInput"
  />
</template>
