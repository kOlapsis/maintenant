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
defineProps<{
  percentage: number | null
  label?: string
}>()

function barColor(pct: number): string {
  if (pct >= 99) return 'bg-pb-sev-ok-solid'
  if (pct >= 95) return 'bg-yellow-500'
  return 'bg-pb-sev-incident-solid'
}
</script>

<template>
  <div class="flex items-center gap-2">
    <span v-if="label" class="w-8 text-xs text-pb-muted">{{ label }}</span>
    <div v-if="percentage !== null" class="flex flex-1 items-center gap-2">
      <div class="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
        <div
          class="h-full rounded-full transition-all"
          :class="barColor(percentage)"
          :style="{ width: `${Math.min(percentage, 100)}%` }"
        />
      </div>
      <span class="w-12 text-right text-xs font-medium text-pb-muted">
        {{ percentage.toFixed(1) }}%
      </span>
    </div>
    <span v-else class="text-xs text-pb-muted">--</span>
  </div>
</template>
