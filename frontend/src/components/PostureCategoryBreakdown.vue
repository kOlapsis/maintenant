<script setup lang="ts">
import type { CategoryScore } from '@/services/postureApi'

defineProps<{
  categories: CategoryScore[]
}>()

const barColorMap: Record<string, string> = {
  green: 'bg-emerald-500',
  yellow: 'bg-amber-500',
  orange: 'bg-orange-500',
  red: 'bg-red-500',
}

const textColorMap: Record<string, string> = {
  green: 'text-emerald-400',
  yellow: 'text-amber-400',
  orange: 'text-orange-400',
  red: 'text-red-400',
}

function scoreColor(score: number): string {
  if (score >= 80) return 'green'
  if (score >= 60) return 'yellow'
  if (score >= 40) return 'orange'
  return 'red'
}

function categoryLabel(name: string): string {
  const labels: Record<string, string> = {
    tls: 'TLS Certificates',
    cves: 'Vulnerabilities (CVEs)',
    updates: 'Available Updates',
    network_exposure: 'Network Exposure',
    image_age: 'Image Age',
  }
  return labels[name] || name
}
</script>

<template>
  <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
    <div
      v-for="cat in categories"
      :key="cat.name"
      class="bg-pb-surface rounded-xl p-4 border border-slate-800"
    >
      <div class="mb-2 flex items-center justify-between">
        <span class="text-sm font-semibold text-pb-primary">
          {{ categoryLabel(cat.name) }}
        </span>
        <span class="text-[10px] text-slate-600 font-bold">{{ cat.weight }}%</span>
      </div>

      <template v-if="cat.applicable">
        <div class="mb-2 flex items-baseline gap-1.5">
          <span class="text-2xl font-black" :class="textColorMap[scoreColor(cat.sub_score)]">
            {{ cat.sub_score }}
          </span>
          <span class="text-[10px] text-slate-600 font-bold">/100</span>
        </div>

        <div class="mb-2 h-1.5 w-full overflow-hidden rounded-full bg-pb-primary border border-slate-800">
          <div
            class="h-full rounded-full transition-all duration-700"
            :class="barColorMap[scoreColor(cat.sub_score)]"
            :style="{ width: cat.sub_score + '%' }"
          />
        </div>

        <div class="flex items-center justify-between text-[10px]">
          <span class="text-slate-500">{{ cat.summary }}</span>
          <span v-if="cat.issue_count > 0" class="text-slate-400 font-bold">
            {{ cat.issue_count }} issue{{ cat.issue_count !== 1 ? 's' : '' }}
          </span>
        </div>
      </template>

      <template v-else>
        <div class="py-2 text-xs text-slate-600 italic">
          Not applicable
        </div>
      </template>
    </div>
  </div>
</template>
