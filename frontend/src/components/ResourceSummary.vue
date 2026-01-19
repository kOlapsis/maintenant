<script setup lang="ts">
import { computed, ref } from 'vue'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import TopConsumersWidget, { type TopConsumer } from './TopConsumersWidget.vue'

const store = useResourcesStore()
const containersStore = useContainersStore()

const topMetric = ref<'cpu' | 'memory'>('cpu')

const totalMemUsed = computed(() => {
  return Object.values(store.snapshots).reduce((sum, s) => sum + s.mem_used, 0)
})

const totalMemLimit = computed(() => {
  return Object.values(store.snapshots).reduce((sum, s) => sum + s.mem_limit, 0)
})

const containerCount = computed(() => Object.keys(store.snapshots).length)

function containerName(id: number): string {
  const c = containersStore.activeContainers.find((c) => c.id === id)
  return c?.name || `#${id}`
}

const topConsumers = computed<TopConsumer[]>(() => {
  const entries = Object.entries(store.snapshots)
  if (entries.length === 0) return []

  const sorted = entries
    .map(([idStr, snap]) => ({
      containerId: Number(idStr),
      containerName: containerName(Number(idStr)),
      value: topMetric.value === 'cpu' ? snap.cpu_percent : snap.mem_used,
      percent: topMetric.value === 'cpu'
        ? snap.cpu_percent
        : (snap.mem_limit > 0 ? (snap.mem_used / snap.mem_limit) * 100 : 0),
      rank: 0,
    }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 5)

  sorted.forEach((c, i) => { c.rank = i + 1 })
  return sorted
})

</script>

<template>
  <div
    v-if="containerCount > 0"
    class="mb-6 rounded-lg p-4"
    :style="{
      backgroundColor: 'var(--pb-bg-surface)',
      border: '1px solid var(--pb-border-default)',
      borderRadius: 'var(--pb-radius-lg)',
      boxShadow: 'var(--pb-shadow-card)',
    }"
  >
    <!-- Summary text -->
    <div class="mb-3 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <span>{{ store.formatBytes(totalMemUsed) }} / {{ store.formatBytes(totalMemLimit) }} RAM</span>
      <span>{{ containerCount }} containers</span>
    </div>

    <!-- Top consumers -->
    <div>
      <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Top Consumers</h4>
      <TopConsumersWidget
        :metric="topMetric"
        :consumers="topConsumers"
        @update:metric="topMetric = $event"
      />
    </div>
  </div>
</template>
