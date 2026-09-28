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
export type CountTone = 'neutral' | 'warn' | 'danger' | 'ok'

withDefaults(
  defineProps<{
    value: number | string
    tone?: CountTone
  }>(),
  { tone: 'neutral' },
)

const toneClasses: Record<CountTone, string> = {
  neutral: 'bg-mnt-elevated text-mnt-muted',
  warn: 'bg-mnt-status-warn text-mnt-status-warn',
  danger: 'bg-mnt-status-down text-mnt-status-down',
  ok: 'bg-mnt-status-ok text-mnt-status-ok',
}
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-1.5 py-0.5 text-xs font-medium tabular-nums"
    :class="toneClasses[tone]"
  >
    {{ value }}
  </span>
</template>
