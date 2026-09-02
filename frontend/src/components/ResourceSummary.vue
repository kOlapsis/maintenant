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
import { getTopConsumers } from '@/services/resourceApi'
import TopConsumersWidget, { type TopConsumer, type Period } from './TopConsumersWidget.vue'
import CollapsiblePanel from '@/components/ui/CollapsiblePanel.vue'

const store = useResourcesStore()

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

const leader = computed(() => topConsumers.value.find((c) => c.rank === 1) ?? topConsumers.value[0] ?? null)

const leaderValue = computed(() => {
  const c = leader.value
  if (!c) return ''
  return topMetric.value === 'cpu' ? `${c.value.toFixed(1)}%` : store.formatBytes(c.value)
})
</script>

<template>
  <CollapsiblePanel v-if="containerCount > 0" storage-key="top-consumers" title="Top consumers">
    <template #summary>
      <span class="truncate">
        {{ store.formatBytes(totalMemUsed) }} / {{ store.formatBytes(totalMemLimit) }} RAM
        &middot; {{ containerCount }} containers
        <template v-if="leader">&middot; {{ leader.containerName }} {{ leaderValue }}</template>
      </span>
    </template>

    <div class="mb-3 flex items-center justify-between text-xs text-mnt-muted">
      <span>{{ store.formatBytes(totalMemUsed) }} / {{ store.formatBytes(totalMemLimit) }} RAM</span>
      <span>{{ containerCount }} containers</span>
    </div>

    <TopConsumersWidget
      :metric="topMetric"
      :period="topPeriod"
      :consumers="topConsumers"
      @update:metric="topMetric = $event"
      @update:period="topPeriod = $event"
    />
  </CollapsiblePanel>
</template>
