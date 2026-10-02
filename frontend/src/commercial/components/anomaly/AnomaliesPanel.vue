<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { Loader2, LineChart } from 'lucide-vue-next'
import { useAnomaliesStore } from '@/commercial/stores/anomalies'
import { useContainersStore } from '@/stores/containers'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { findContainerForScope } from '@/commercial/utils/anomalyScope'
import UiButton from '@/components/ui/UiButton.vue'
import LearningScreen from './LearningScreen.vue'
import AnomalyFeed from './AnomalyFeed.vue'
import SeriesDetail from './SeriesDetail.vue'
import SeasonalityTuner from './SeasonalityTuner.vue'
import OccurrenceHistory from './OccurrenceHistory.vue'
import type { AnomalyEventItem, SeriesItem } from '@/commercial/services/anomalyApi'

const store = useAnomaliesStore()
const containersStore = useContainersStore()
const slideOver = inject(detailSlideOverKey, null)

const selectedScope = ref<string | null>(null)
const selectedMetric = ref<string | null>(null)

const alertSeverity = computed(() => store.settings?.alert_severity)

const containerSeries = computed(() => store.series.filter((s) => s.scope_type === 'container'))

const scopeGroups = computed(() => {
  const m = new Map<string, SeriesItem[]>()
  for (const s of containerSeries.value) {
    const arr = m.get(s.scope_id) ?? []
    arr.push(s)
    m.set(s.scope_id, arr)
  }
  return m
})

const selectedMetrics = computed<SeriesItem[]>(() => {
  if (!selectedScope.value) return []
  return scopeGroups.value.get(selectedScope.value) ?? []
})

const selectedAnomalousMetrics = computed<string[]>(() => {
  if (!selectedScope.value) return []
  return store.events
    .filter((e) => e.scope_id === selectedScope.value && e.ended_at == null)
    .map((e) => e.metric)
})

const selectedOccurrences = computed<AnomalyEventItem[]>(() => {
  if (!selectedScope.value) return []
  return store.events.filter(
    (e) => e.scope_id === selectedScope.value && (!selectedMetric.value || e.metric === selectedMetric.value),
  )
})

const linkedContainer = computed(() =>
  selectedScope.value ? findContainerForScope(containersStore.allContainers, selectedScope.value) : null,
)

function onSelectEvent(ev: AnomalyEventItem) {
  selectedScope.value = ev.scope_id
  selectedMetric.value = ev.metric
}

function openContainerCharts() {
  if (linkedContainer.value && slideOver) {
    slideOver.openDetail('container', linkedContainer.value.id)
  }
}

onMounted(() => {
  if (!store.loaded) store.load()
})
</script>

<template>
  <div v-if="store.loading && !store.loaded" class="flex items-center justify-center py-16 text-mnt-muted">
    <Loader2 :size="20" class="mr-2 animate-spin" /> Loading…
  </div>

  <div v-else-if="store.error" class="rounded-xl border border-mnt-default bg-mnt-surface p-6 text-sm text-mnt-muted">
    {{ store.error }}
  </div>

  <LearningScreen v-else-if="!store.anyReady" :series="containerSeries" />

  <div v-else class="grid grid-cols-1 gap-6 lg:grid-cols-3">
    <div class="lg:col-span-2">
      <AnomalyFeed :events="store.events" :alert-severity="alertSeverity" @select="onSelectEvent" />
    </div>
    <div class="space-y-4">
      <template v-if="selectedScope && selectedMetrics.length">
        <SeriesDetail
          :scope-id="selectedScope"
          :metrics="selectedMetrics"
          :anomalous-metrics="selectedAnomalousMetrics"
          :alert-severity="alertSeverity"
        />

        <OccurrenceHistory :events="selectedOccurrences" />

        <UiButton v-if="linkedContainer" variant="secondary" block :icon="LineChart" @click="openContainerCharts">
          Open {{ linkedContainer.name }} charts
        </UiButton>
      </template>
      <div v-else class="rounded-xl border border-mnt-default bg-mnt-surface p-6 text-sm text-mnt-muted">
        Select an anomaly to inspect its series configuration and per-metric state.
      </div>

      <SeasonalityTuner
        v-if="store.settings"
        :settings="store.settings"
        :saving="store.savingSettings"
        @change="store.saveBucketPull"
      />
    </div>
  </div>
</template>
