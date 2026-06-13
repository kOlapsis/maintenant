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
  uptimes: Record<string, number>
}>()

function uptimeColor(pct: number): string {
  if (pct >= 99) return 'bg-pb-sev-ok-solid'
  if (pct >= 95) return 'bg-yellow-500'
  return 'bg-pb-sev-incident-solid'
}

function formatPct(pct: number): string {
  if (pct === 0) return '-'
  return pct.toFixed(1) + '%'
}

const windowLabels: Record<string, string> = {
  '1h': '1h',
  '24h': '24h',
  '7d': '7d',
  '30d': '30d',
}
</script>

<template>
  <div class="flex items-center gap-3">
    <div
      v-for="(label, key) in windowLabels"
      :key="key"
      class="flex items-center gap-1.5 text-xs"
    >
      <span class="text-pb-muted">{{ label }}</span>
      <div class="h-2 w-12 rounded-full bg-gray-200 overflow-hidden">
        <div
          class="h-full rounded-full transition-all"
          :class="uptimeColor(uptimes[key] || 0)"
          :style="{ width: Math.min(uptimes[key] || 0, 100) + '%' }"
        />
      </div>
      <span class="text-pb-muted tabular-nums">{{ formatPct(uptimes[key] || 0) }}</span>
    </div>
  </div>
</template>
