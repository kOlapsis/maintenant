<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
defineProps<{
  percentage: number | null
  label?: string
}>()

function barColor(pct: number): string {
  if (pct >= 99) return 'bg-mnt-sev-ok-solid'
  if (pct >= 95) return 'bg-yellow-500'
  return 'bg-mnt-sev-incident-solid'
}
</script>

<template>
  <div class="flex items-center gap-2">
    <span v-if="label" class="w-8 text-xs text-mnt-muted">{{ label }}</span>
    <div v-if="percentage !== null" class="flex flex-1 items-center gap-2">
      <div class="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
        <div
          class="h-full rounded-full transition-all"
          :class="barColor(percentage)"
          :style="{ width: `${Math.min(percentage, 100)}%` }"
        />
      </div>
      <span class="w-12 text-right text-xs font-medium text-mnt-muted">
        {{ percentage.toFixed(1) }}%
      </span>
    </div>
    <span v-else class="text-xs text-mnt-muted">--</span>
  </div>
</template>
