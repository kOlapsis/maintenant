<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed } from 'vue'
import type { SeriesItem } from '@/commercial/services/anomalyApi'
import AnomalyBadge from './AnomalyBadge.vue'

const props = defineProps<{
  scopeId: string
  metrics: SeriesItem[]
  anomalousMetrics?: string[]
  alertSeverity?: string
}>()

const cfg = computed(() => props.metrics[0])
const enabledMetrics = computed(() => props.metrics.map((m) => m.metric))
const optedOut = computed(() => props.metrics.every((m) => m.state === 'disabled'))
const anomalous = computed(() => new Set(props.anomalousMetrics ?? []))
const scopeAnomalous = computed(() => props.metrics.some((m) => anomalous.value.has(m.metric)))
</script>

<template>
  <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
    <div class="flex items-center justify-between gap-3">
      <h3 class="min-w-0 truncate font-mono text-sm text-mnt-primary">{{ scopeId }}</h3>
      <AnomalyBadge
        v-if="cfg"
        :state="cfg.state"
        :progress="cfg.progress"
        :active="scopeAnomalous"
        :alert-severity="alertSeverity"
      />
    </div>

    <dl class="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
      <div>
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Sensitivity</dt>
        <dd class="mt-0.5 capitalize text-mnt-secondary">{{ cfg?.sensitivity ?? '—' }}</dd>
      </div>
      <div>
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Opt-out</dt>
        <dd class="mt-0.5 text-mnt-secondary">{{ optedOut ? 'Yes' : 'No' }}</dd>
      </div>
      <div class="col-span-2">
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Monitored metrics</dt>
        <dd class="mt-1 flex flex-wrap gap-1.5">
          <span
            v-for="m in enabledMetrics"
            :key="m"
            class="rounded bg-mnt-elevated px-2 py-0.5 text-xs text-mnt-secondary"
          >
            {{ m }}
          </span>
        </dd>
      </div>
    </dl>

    <ul class="mt-4 space-y-1.5">
      <li
        v-for="m in metrics"
        :key="m.metric + m.dimension"
        class="flex items-center justify-between gap-2 rounded-lg bg-mnt-elevated px-3 py-1.5 text-sm"
      >
        <span class="text-mnt-secondary">{{ m.metric }}<span v-if="m.dimension" class="text-mnt-muted"> · {{ m.dimension }}</span></span>
        <AnomalyBadge
          :state="m.state"
          :progress="m.progress"
          :active="anomalous.has(m.metric)"
          :alert-severity="alertSeverity"
        />
      </li>
    </ul>
  </div>
</template>
