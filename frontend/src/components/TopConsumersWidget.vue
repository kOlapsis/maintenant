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
import { computed, ref } from 'vue'
import { useEdition } from '@/composables/useEdition'
import { Lock } from 'lucide-vue-next'
import SegmentedToggle from './ui/SegmentedToggle.vue'

/**
 * A period is whatever the engine's catalogue declares. It used to be a fixed
 * union split into a free list and a paid one, which is how the widget ended up
 * being the only thing standing between an edition and a paid period.
 */
export type Period = string

export interface TopConsumer {
  containerId: string
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

const { historyWindows, isWindowOpen } = useEdition()

const activeMetric = ref<'cpu' | 'memory'>(props.metric)
const activePeriod = ref<Period>(props.period)

function switchMetric(m: 'cpu' | 'memory') {
  activeMetric.value = m
  emit('update:metric', m)
}

function switchPeriod(p: Period) {
  // The server refuses a closed period anyway; not asking is what keeps the
  // interface from firing a request it knows will come back refused.
  if (!isWindowOpen(p)) return
  activePeriod.value = p
  emit('update:period', p)
}

const periodOptions = computed(() =>
  historyWindows.value.map((w) => ({
    value: w.window,
    label: w.window,
    icon: isWindowOpen(w.window) ? undefined : Lock,
    disabled: !isWindowOpen(w.window),
    locked: !isWindowOpen(w.window),
    dataTest: `period-${w.window}`,
  })),
)

function barColor(percent: number): string {
  if (percent >= 90) return 'var(--mnt-status-down)'
  if (percent >= 70) return 'var(--mnt-status-warn)'
  return 'var(--mnt-status-ok)'
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
      <SegmentedToggle
        :model-value="activeMetric"
        :options="[{ value: 'cpu', label: 'CPU' }, { value: 'memory', label: 'Memory' }]"
        ariaLabel="Metric"
        @update:model-value="switchMetric"
      />

      <div
        class="h-4 w-px"
        :style="{ backgroundColor: 'var(--mnt-border-default)' }"
      />

      <SegmentedToggle
        :model-value="activePeriod"
        :options="periodOptions"
        ariaLabel="Period"
        @update:model-value="switchPeriod"
      />
    </div>

    <!-- Ranked list -->
    <div v-if="consumers.length === 0" class="text-xs" :style="{ color: 'var(--mnt-text-muted)' }">
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
          :style="{ color: 'var(--mnt-text-muted)' }"
        >
          {{ consumer.rank }}
        </span>

        <!-- Name + bar -->
        <div class="min-w-0 flex-1">
          <div class="mb-0.5 truncate text-xs font-medium" :style="{ color: 'var(--mnt-text-primary)' }">
            {{ consumer.containerName }}
          </div>
          <div
            class="h-1.5 w-full rounded-full"
            :style="{ backgroundColor: 'var(--mnt-bg-elevated)' }"
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
        <span class="shrink-0 text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">
          {{ formatValue(consumer) }}
        </span>
      </div>
    </div>
  </div>
</template>
