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

withDefaults(
  defineProps<{
    rows?: number
    invalid?: boolean
    mono?: boolean
  }>(),
  { rows: 3, invalid: false, mono: false },
)

const model = defineModel<string>({ default: '' })
</script>

<template>
  <textarea
    v-bind="$attrs"
    v-model="model"
    :rows="rows"
    :aria-invalid="invalid || undefined"
    class="focus-ring w-full rounded-lg border bg-mnt-elevated px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted disabled:cursor-not-allowed disabled:opacity-60"
    :class="[invalid ? 'border-[var(--mnt-status-down-text)]' : 'border-mnt-default', mono ? 'font-mono' : '']"
  />
</template>
