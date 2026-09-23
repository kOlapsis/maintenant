<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
