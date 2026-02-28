<script setup lang="ts">
defineProps<{
  percentage: number | null
  label?: string
}>()

function barColor(pct: number): string {
  if (pct >= 99) return 'bg-green-500'
  if (pct >= 95) return 'bg-yellow-500'
  return 'bg-red-500'
}
</script>

<template>
  <div class="flex items-center gap-2">
    <span v-if="label" class="w-8 text-xs text-gray-500">{{ label }}</span>
    <div v-if="percentage !== null" class="flex flex-1 items-center gap-2">
      <div class="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
        <div
          class="h-full rounded-full transition-all"
          :class="barColor(percentage)"
          :style="{ width: `${Math.min(percentage, 100)}%` }"
        />
      </div>
      <span class="w-12 text-right text-xs font-medium text-gray-600">
        {{ percentage.toFixed(1) }}%
      </span>
    </div>
    <span v-else class="text-xs text-gray-400">--</span>
  </div>
</template>
