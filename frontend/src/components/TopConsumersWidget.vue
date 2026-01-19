<script setup lang="ts">
import { ref } from 'vue'

export interface TopConsumer {
  containerId: number
  containerName: string
  value: number
  percent: number
  rank: number
}

const props = defineProps<{
  metric: 'cpu' | 'memory'
  consumers: TopConsumer[]
}>()

const emit = defineEmits<{
  'update:metric': [value: 'cpu' | 'memory']
}>()

const activeMetric = ref<'cpu' | 'memory'>(props.metric)

function switchMetric(m: 'cpu' | 'memory') {
  activeMetric.value = m
  emit('update:metric', m)
}

function barColor(percent: number): string {
  if (percent >= 90) return 'var(--pb-status-down)'
  if (percent >= 70) return 'var(--pb-status-warn)'
  return 'var(--pb-status-ok)'
}

function formatValue(consumer: TopConsumer): string {
  if (activeMetric.value === 'cpu') {
    return `${consumer.value.toFixed(1)}%`
  }
  // Memory in bytes
  const bytes = consumer.value
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(0)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}
</script>

<template>
  <div>
    <!-- Metric toggle tabs -->
    <div class="mb-3 flex gap-1">
      <button
        v-for="m in (['cpu', 'memory'] as const)"
        :key="m"
        class="rounded-full px-3 py-1 text-xs font-medium transition"
        :style="{
          backgroundColor: activeMetric === m ? 'var(--pb-accent)' : 'var(--pb-bg-elevated)',
          color: activeMetric === m ? 'var(--pb-text-inverted)' : 'var(--pb-text-secondary)',
          border: activeMetric === m ? '1px solid var(--pb-accent)' : '1px solid var(--pb-border-default)',
        }"
        @click="switchMetric(m)"
      >
        {{ m === 'cpu' ? 'CPU' : 'Memory' }}
      </button>
    </div>

    <!-- Ranked list -->
    <div v-if="consumers.length === 0" class="text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      No resource data available.
    </div>
    <div v-else class="space-y-2">
      <div
        v-for="consumer in consumers.slice(0, 5)"
        :key="consumer.containerId"
        class="flex items-center gap-2"
      >
        <!-- Rank -->
        <span
          class="w-5 text-center text-xs font-semibold"
          :style="{ color: 'var(--pb-text-muted)' }"
        >
          {{ consumer.rank }}
        </span>

        <!-- Name + bar -->
        <div class="min-w-0 flex-1">
          <div class="mb-0.5 truncate text-xs font-medium" :style="{ color: 'var(--pb-text-primary)' }">
            {{ consumer.containerName }}
          </div>
          <div
            class="h-1.5 w-full rounded-full"
            :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
          >
            <div
              class="h-1.5 rounded-full transition-all"
              :style="{
                width: Math.min(consumer.percent, 100) + '%',
                backgroundColor: barColor(consumer.percent),
              }"
            />
          </div>
        </div>

        <!-- Value -->
        <span class="shrink-0 text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">
          {{ formatValue(consumer) }}
        </span>
      </div>
    </div>
  </div>
</template>
