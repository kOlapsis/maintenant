<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { Activity } from 'lucide-vue-next'
import { severityFromAlert, severityVar, type Severity } from '@/composables/useSeverity'
import { formatMetricValue } from '@/commercial/utils/metricFormat'
import type { AnomalyEventItem } from '@/commercial/services/anomalyApi'

const props = defineProps<{
  events: AnomalyEventItem[]
  alertSeverity?: string
}>()

const emit = defineEmits<{ (e: 'select', ev: AnomalyEventItem): void }>()

const ordered = computed(() =>
  [...props.events].sort((a, b) => {
    const aActive = a.ended_at == null ? 1 : 0
    const bActive = b.ended_at == null ? 1 : 0
    if (aActive !== bActive) return bActive - aActive
    return b.started_at - a.started_at
  }),
)

const detectorLabel: Record<string, string> = {
  spike: 'Spike',
  drift: 'Drift',
  change_point: 'Level shift',
}

const tierLabel: Record<string, string> = {
  active: 'alert raised',
  passive: 'flagged only',
}

function dotSeverity(ev: AnomalyEventItem): Severity {
  if (ev.ended_at != null) return 'ok'
  return ev.tier === 'active' ? severityFromAlert(props.alertSeverity ?? '') : 'neutral'
}

function duration(ev: AnomalyEventItem): string {
  const end = ev.ended_at ?? Math.floor(Date.now() / 1000)
  const secs = Math.max(0, end - ev.started_at)
  if (secs < 60) return `${secs}s`
  if (secs < 3600) return `${Math.round(secs / 60)}m`
  return `${Math.round(secs / 3600)}h`
}

function fmtTime(ts: number): string {
  return new Date(ts * 1000).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="rounded-xl border border-mnt-default bg-mnt-surface">
    <header class="flex items-center gap-2 border-b border-mnt-subtle px-4 py-3">
      <Activity :size="16" class="text-mnt-muted" />
      <h2 class="text-sm font-semibold text-mnt-primary">Anomaly feed</h2>
      <span class="ml-auto text-xs text-mnt-muted">{{ ordered.length }} total</span>
    </header>

    <p v-if="ordered.length === 0" class="px-4 py-8 text-center text-sm text-mnt-muted">
      All nominal: no anomalies detected.
    </p>

    <ul v-else>
      <li
        v-for="ev in ordered"
        :key="ev.id"
        class="feed-row flex cursor-pointer items-center gap-3 border-t border-mnt-subtle px-4 py-3 first:border-t-0"
        @click="emit('select', ev)"
      >
        <span class="h-2 w-2 shrink-0 rounded-full" :style="{ backgroundColor: severityVar(dotSeverity(ev)) }" />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate font-mono text-xs text-mnt-secondary">{{ ev.scope_id }}</span>
            <span class="rounded bg-mnt-elevated px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-mnt-muted">{{ ev.metric }}</span>
          </div>
          <div class="mt-0.5 text-xs text-mnt-muted">
            {{ detectorLabel[ev.detector] ?? ev.detector }} · {{ tierLabel[ev.tier] ?? ev.tier }} · started {{ fmtTime(ev.started_at) }}
          </div>
        </div>
        <div class="shrink-0 text-right">
          <div class="text-xs font-medium text-mnt-primary">
            peak {{ formatMetricValue(ev.metric, ev.peak_value) }}
          </div>
          <div class="text-[11px] text-mnt-muted">
            {{ ev.ended_at == null ? `ongoing for ${duration(ev)}` : `lasted ${duration(ev)}` }}
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.feed-row {
  transition: background-color 0.15s;
}
.feed-row:hover {
  background-color: var(--mnt-bg-hover);
}
</style>
