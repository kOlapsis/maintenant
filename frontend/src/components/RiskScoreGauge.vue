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

const props = defineProps<{
  score: number
  level: string
}>()

const gaugeColor = computed(() => {
  if (props.score >= 81) return { bar: 'bg-rose-500', text: 'text-rose-400' }
  if (props.score >= 61) return { bar: 'bg-orange-500', text: 'text-orange-400' }
  if (props.score >= 31) return { bar: 'bg-amber-500', text: 'text-amber-400' }
  return { bar: 'bg-emerald-500', text: 'text-emerald-400' }
})

const levelLabel = computed(() => {
  switch (props.level) {
    case 'critical': return 'Critical'
    case 'high': return 'High'
    case 'moderate': return 'Moderate'
    case 'low': return 'Low'
    default: return props.level
  }
})
</script>

<template>
  <div class="space-y-1.5">
    <div class="flex items-baseline justify-between">
      <span :class="['text-2xl font-black', gaugeColor.text]">{{ score }}</span>
      <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">{{ levelLabel }}</span>
    </div>
    <div class="h-2 w-full bg-pb-primary rounded-full border border-slate-800 overflow-hidden">
      <div
        class="h-full rounded-full transition-all duration-500"
        :class="gaugeColor.bar"
        :style="{ width: `${Math.min(score, 100)}%` }"
      />
    </div>
  </div>
</template>
