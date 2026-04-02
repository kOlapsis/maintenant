<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  score: number
  color: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
  label?: string
}>()

const isXs = computed(() => props.size === 'xs')

const sizes = {
  xs: { ring: '', text: '' },
  sm: { ring: 'h-8 w-8', text: 'text-xs font-semibold' },
  md: { ring: 'h-14 w-14', text: 'text-lg font-bold' },
  lg: { ring: 'h-24 w-24', text: 'text-3xl font-bold' },
} as const

const colorMap: Record<string, string> = {
  green: 'border-emerald-500 text-pb-status-ok',
  yellow: 'border-amber-500 text-amber-400',
  orange: 'border-orange-500 text-orange-400',
  red: 'border-red-500 text-red-400',
}

const bgMap: Record<string, string> = {
  green: 'bg-emerald-500/10',
  yellow: 'bg-amber-500/10',
  orange: 'bg-orange-500/10',
  red: 'bg-red-500/10',
}

const xsColorMap: Record<string, string> = {
  green: 'text-pb-status-ok',
  yellow: 'text-amber-400',
  orange: 'text-orange-400',
  red: 'text-red-400',
}

const xsBgMap: Record<string, string> = {
  green: 'bg-emerald-500/10',
  yellow: 'bg-amber-500/10',
  orange: 'bg-orange-500/10',
  red: 'bg-red-500/10',
}

const s = computed(() => sizes[props.size ?? 'md'])
const borderColor = computed(() => colorMap[props.color] ?? colorMap.red)
const bgColor = computed(() => bgMap[props.color] ?? bgMap.red)
const xsTextColor = computed(() => xsColorMap[props.color] ?? xsColorMap.red)
const xsBg = computed(() => xsBgMap[props.color] ?? xsBgMap.red)
</script>

<template>
  <!-- xs: inline pill badge -->
  <span
    v-if="isXs"
    class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-bold"
    :class="[xsTextColor, xsBg]"
    :title="`Security score: ${score}/100`"
  >{{ score }}</span>

  <!-- sm/md/lg: circular ring -->
  <div v-else class="flex flex-col items-center gap-1">
    <div
      class="flex items-center justify-center rounded-full border-3"
      :class="[s.ring, borderColor, bgColor]"
    >
      <span :class="s.text">{{ score }}</span>
    </div>
    <span v-if="label" class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">{{ label }}</span>
  </div>
</template>
