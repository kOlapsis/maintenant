<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { ref } from 'vue'
import { useEdition } from '@/composables/useEdition'
import { Lock } from 'lucide-vue-next'

export type Period = '1h' | '24h' | '7d' | '30d'

export interface TopConsumer {
  containerId: number
  containerName: string
  value: number
  percent: number
  rank: number
}

const props = defineProps<{
  metric: 'cpu' | 'memory'
  period: Period
  consumers: TopConsumer[]
}>()

const emit = defineEmits<{
  'update:metric': [value: 'cpu' | 'memory']
  'update:period': [value: Period]
}>()

const { hasFeature } = useEdition()

const cePeriods: Period[] = ['1h']
const proPeriods: Period[] = ['24h', '7d', '30d']

const activeMetric = ref<'cpu' | 'memory'>(props.metric)
const activePeriod = ref<Period>(props.period)

function switchMetric(m: 'cpu' | 'memory') {
  activeMetric.value = m
  emit('update:metric', m)
}

function switchPeriod(p: Period) {
  if (proPeriods.includes(p) && !hasFeature('resource_history')) return
  activePeriod.value = p
  emit('update:period', p)
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
    <!-- Metric + period toggles -->
    <div class="mb-3 flex items-center gap-3">
      <div class="flex gap-1">
        <button
          v-for="m in (['cpu', 'memory'] as const)"
          :key="m"
          class="rounded-full px-3 py-1 text-xs font-medium transition cursor-pointer"
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

      <div
        class="h-4 w-px"
        :style="{ backgroundColor: 'var(--pb-border-default)' }"
      />

      <div class="flex gap-1">
        <button
          v-for="p in cePeriods"
          :key="p"
          class="rounded-full px-2.5 py-1 text-xs font-medium transition cursor-pointer"
          :style="{
            backgroundColor: activePeriod === p ? 'var(--pb-accent)' : 'var(--pb-bg-elevated)',
            color: activePeriod === p ? 'var(--pb-text-inverted)' : 'var(--pb-text-secondary)',
            border: activePeriod === p ? '1px solid var(--pb-accent)' : '1px solid var(--pb-border-default)',
          }"
          @click="switchPeriod(p)"
        >
          {{ p }}
        </button>
        <button
          v-for="p in proPeriods"
          :key="p"
          class="rounded-full px-2.5 py-1 text-xs font-medium transition flex items-center gap-1"
          :style="{
            backgroundColor: hasFeature('resource_history') && activePeriod === p ? 'var(--pb-accent)' : 'var(--pb-bg-elevated)',
            color: hasFeature('resource_history') && activePeriod === p ? 'var(--pb-text-inverted)' : hasFeature('resource_history') ? 'var(--pb-text-secondary)' : 'var(--pb-text-muted)',
            border: hasFeature('resource_history') && activePeriod === p ? '1px solid var(--pb-accent)' : '1px solid var(--pb-border-default)',
            cursor: hasFeature('resource_history') ? 'pointer' : 'not-allowed',
            opacity: hasFeature('resource_history') ? '1' : '0.5',
          }"
          @click="switchPeriod(p)"
        >
          {{ p }}
          <Lock v-if="!hasFeature('resource_history')" :size="9" />
        </button>
      </div>
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
