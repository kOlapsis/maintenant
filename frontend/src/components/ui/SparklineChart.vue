<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    data: number[]
    width?: number
    height?: number
    color?: string
    fillOpacity?: number
    highlightLast?: boolean
  }>(),
  {
    width: 80,
    height: 24,
    color: 'var(--pb-accent)',
    fillOpacity: 0.1,
    highlightLast: true,
  },
)

const points = computed(() => {
  if (!props.data.length) return ''
  const max = Math.max(...props.data)
  const min = Math.min(...props.data)
  const range = max - min || 1
  const step = props.width / Math.max(props.data.length - 1, 1)

  return props.data
    .map((v, i) => {
      const x = i * step
      const y = props.height - ((v - min) / range) * (props.height - 4) - 2
      return `${x},${y}`
    })
    .join(' ')
})

const fillPoints = computed(() => {
  if (!points.value) return ''
  return `0,${props.height} ${points.value} ${props.width},${props.height}`
})

const lastPoint = computed(() => {
  if (!props.data.length) return null
  const max = Math.max(...props.data)
  const min = Math.min(...props.data)
  const range = max - min || 1
  const step = props.width / Math.max(props.data.length - 1, 1)
  const i = props.data.length - 1
  const val = props.data[i] ?? 0
  return {
    x: i * step,
    y: props.height - ((val - min) / range) * (props.height - 4) - 2,
  }
})
</script>

<template>
  <svg
    v-if="data.length > 1"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    preserveAspectRatio="none"
    class="inline-block"
  >
    <polygon
      v-if="fillOpacity > 0"
      :points="fillPoints"
      :fill="color"
      :fill-opacity="fillOpacity"
    />
    <polyline
      :points="points"
      fill="none"
      :stroke="color"
      stroke-width="1.5"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
    <circle
      v-if="highlightLast && lastPoint"
      :cx="lastPoint.x"
      :cy="lastPoint.y"
      r="2"
      :fill="color"
    />
  </svg>
</template>
