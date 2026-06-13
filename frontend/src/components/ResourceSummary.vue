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
import { computed, ref, watch, onMounted } from 'vue'
import { useResourcesStore } from '@/stores/resources'
import { useContainersStore } from '@/stores/containers'
import { getTopConsumers } from '@/services/resourceApi'
import TopConsumersWidget, { type TopConsumer, type Period } from './TopConsumersWidget.vue'

const store = useResourcesStore()
const containersStore = useContainersStore()

const topMetric = ref<'cpu' | 'memory'>('cpu')
const topPeriod = ref<Period>('1h')
const topConsumers = ref<TopConsumer[]>([])
const loading = ref(false)

const totalMemUsed = computed(() => {
  return store.summary?.total_mem_used ?? 0
})

const totalMemLimit = computed(() => {
  return store.summary?.total_mem_limit ?? 0
})

const containerCount = computed(() => Object.keys(store.snapshots).length)

async function fetchTopConsumers() {
  loading.value = true
  try {
    const resp = await getTopConsumers(topMetric.value, topPeriod.value, 5, store.summaryQuery)
    topConsumers.value = resp.consumers.map((c) => ({
      containerId: c.container_id,
      containerName: c.container_name,
      value: c.value,
      percent: c.percent,
      rank: c.rank,
    }))
  } catch {
    topConsumers.value = []
  } finally {
    loading.value = false
  }
}

onMounted(fetchTopConsumers)
watch([topMetric, topPeriod, () => store.summaryQuery], fetchTopConsumers)
</script>

<template>
  <div
    v-if="containerCount > 0"
    class="mb-6 rounded-lg p-4"
    :style="{
      backgroundColor: 'var(--mnt-bg-surface)',
      border: '1px solid var(--mnt-border-default)',
      borderRadius: 'var(--mnt-radius-lg)',
      boxShadow: 'var(--mnt-shadow-card)',
    }"
  >
    <!-- Summary text -->
    <div class="mb-3 flex items-center justify-between text-xs" :style="{ color: 'var(--mnt-text-muted)' }">
      <span>{{ store.formatBytes(totalMemUsed) }} / {{ store.formatBytes(totalMemLimit) }} RAM</span>
      <span>{{ containerCount }} containers</span>
    </div>

    <!-- Top consumers -->
    <div>
      <h4 class="mb-2 text-xs font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">Top Consumers</h4>
      <TopConsumersWidget
        :metric="topMetric"
        :period="topPeriod"
        :consumers="topConsumers"
        @update:metric="topMetric = $event"
        @update:period="topPeriod = $event"
      />
    </div>
  </div>
</template>
