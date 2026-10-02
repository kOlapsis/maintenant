<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { History } from 'lucide-vue-next'
import { formatMetricValue } from '@/commercial/utils/metricFormat'
import type { AnomalyEventItem } from '@/commercial/services/anomalyApi'

const props = defineProps<{
  events: AnomalyEventItem[]
}>()

const ordered = computed(() => [...props.events].sort((a, b) => b.started_at - a.started_at))
const latest = computed(() => ordered.value[0])

function startHour(ev: AnomalyEventItem): number {
  return new Date(ev.started_at * 1000).getHours()
}

const pattern = computed<string | null>(() => {
  const list = ordered.value
  const newest = list[0]
  const oldest = list[list.length - 1]
  if (list.length < 3 || !newest || !oldest) return null

  const hours = new Set(list.map(startHour))
  const hoursSpanned = Math.max(1, Math.round((newest.started_at - oldest.started_at) / 3600))
  const window = hoursSpanned >= 48 ? `${Math.round(hoursSpanned / 24)}d` : `${hoursSpanned}h`

  if (hours.size === 1) {
    const h = String([...hours][0]).padStart(2, '0')
    return `${list.length} times in ${window}, always at ${h}:00`
  }
  if (hours.size <= 3) {
    const labels = [...hours].sort((a, b) => a - b).map((h) => `${String(h).padStart(2, '0')}:00`)
    return `${list.length} times in ${window}, always around ${labels.join(', ')}`
  }
  if (list.length >= hoursSpanned * 0.8) {
    return `${list.length} times in ${window}, roughly every hour`
  }
  return `${list.length} times in ${window}`
})

function fmtTime(ts: number): string {
  return new Date(ts * 1000).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function duration(ev: AnomalyEventItem): string {
  const end = ev.ended_at ?? Math.floor(Date.now() / 1000)
  const secs = Math.max(0, end - ev.started_at)
  if (secs < 60) return `${secs}s`
  if (secs < 3600) return `${Math.round(secs / 60)}m`
  return `${Math.round(secs / 3600)}h`
}

const deviation = computed(() => (latest.value ? Math.abs(latest.value.peak_deviation).toFixed(1) : '0'))
</script>

<template>
  <div v-if="ordered.length" class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
    <div class="flex items-center gap-2">
      <History :size="16" class="text-mnt-muted" />
      <h3 class="text-sm font-semibold text-mnt-primary">Occurrences</h3>
      <span class="ml-auto text-xs text-mnt-muted">{{ ordered.length }}</span>
    </div>

    <p v-if="pattern" class="mt-2 rounded-lg bg-mnt-elevated px-3 py-2 text-xs text-mnt-secondary">
      {{ pattern }}
    </p>

    <dl v-if="latest" class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
      <div>
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Peak</dt>
        <dd class="mt-0.5 text-mnt-primary">{{ formatMetricValue(latest.metric, latest.peak_value) }}</dd>
      </div>
      <div>
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Expected</dt>
        <dd class="mt-0.5 text-mnt-secondary">{{ formatMetricValue(latest.metric, latest.baseline_median) }}</dd>
      </div>
      <div class="col-span-2">
        <dt class="text-xs uppercase tracking-wide text-mnt-muted">Deviation</dt>
        <dd class="mt-0.5 text-mnt-secondary">{{ deviation }}× the learned spread</dd>
      </div>
    </dl>

    <ul class="mt-3 space-y-1 text-xs">
      <li
        v-for="ev in ordered.slice(0, 8)"
        :key="ev.id"
        class="flex items-center justify-between gap-2 text-mnt-muted"
      >
        <span :class="ev.ended_at == null ? 'text-mnt-secondary' : ''">{{ fmtTime(ev.started_at) }}</span>
        <span>{{ ev.ended_at == null ? `ongoing, ${duration(ev)}` : duration(ev) }}</span>
      </li>
    </ul>
    <p v-if="ordered.length > 8" class="mt-1 text-[11px] text-mnt-muted">
      and {{ ordered.length - 8 }} more
    </p>
  </div>
</template>
