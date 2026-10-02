<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { GraduationCap } from 'lucide-vue-next'
import type { SeriesItem } from '@/commercial/services/anomalyApi'
import AnomalyBadge from './AnomalyBadge.vue'

const props = defineProps<{
  series: SeriesItem[]
}>()

interface ScopeGroup {
  scopeId: string
  scopeType: string
  progress: number
  state: SeriesItem['state']
  estReadyAt: number | null
}

const scopes = computed<ScopeGroup[]>(() => {
  const byScope = new Map<string, SeriesItem[]>()
  for (const s of props.series) {
    const arr = byScope.get(s.scope_id) ?? []
    arr.push(s)
    byScope.set(s.scope_id, arr)
  }
  const groups: ScopeGroup[] = []
  for (const [scopeId, items] of byScope) {
    const progress = items.reduce((a, s) => a + s.progress, 0) / items.length
    const allReady = items.every((s) => s.state === 'ready')
    const est = items
      .map((s) => s.estimated_ready_at)
      .filter((v): v is number => v != null)
      .sort((a, b) => a - b)[0]
    groups.push({
      scopeId,
      scopeType: items[0]?.scope_type ?? 'container',
      progress,
      state: allReady ? 'ready' : (items[0]?.state ?? 'learning'),
      estReadyAt: est ?? null,
    })
  }
  return groups.sort((a, b) => b.progress - a.progress)
})

const totalScopes = computed(() => scopes.value.length)
const readyScopes = computed(() => scopes.value.filter((s) => s.state === 'ready').length)
const globalProgress = computed(() => {
  if (props.series.length === 0) return 0
  return props.series.reduce((a, s) => a + s.progress, 0) / props.series.length
})
const earliestReadyAt = computed(() => {
  const est = scopes.value
    .map((s) => s.estReadyAt)
    .filter((v): v is number => v != null)
    .sort((a, b) => a - b)[0]
  return est ?? null
})

function fmtDate(ts: number | null): string {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}
function pct(v: number): string {
  return `${Math.round(v * 100)}%`
}
</script>

<template>
  <div class="rounded-xl border border-mnt-default bg-mnt-surface p-6">
    <div class="flex items-start gap-3">
      <GraduationCap :size="22" class="mt-0.5 shrink-0 text-mnt-green-400" />
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold text-mnt-primary">Learning your baselines</h2>
        <p class="mt-1 text-sm text-mnt-muted">
          Maintenant is observing normal behavior for each monitored series before it flags anything. No anomaly
          alerts are raised during learning; static thresholds still apply.
        </p>

        <div class="mt-5">
          <div class="mb-1 flex items-center justify-between text-sm">
            <span class="text-mnt-secondary">{{ readyScopes }} of {{ totalScopes }} scopes ready</span>
            <span class="font-medium text-mnt-primary">{{ pct(globalProgress) }}</span>
          </div>
          <div class="h-2 w-full overflow-hidden rounded-full bg-mnt-elevated">
            <div
              class="h-full rounded-full bg-mnt-green-500 transition-all"
              :style="{ width: pct(globalProgress) }"
            />
          </div>
          <p class="mt-2 text-xs text-mnt-muted">
            Estimated ready by <span class="font-medium text-mnt-secondary">{{ fmtDate(earliestReadyAt) }}</span>
          </p>
        </div>
      </div>
    </div>

    <ul v-if="scopes.length" class="mt-6 space-y-2">
      <li
        v-for="g in scopes"
        :key="g.scopeId"
        class="flex items-center gap-3 rounded-lg border border-mnt-subtle bg-mnt-elevated px-3 py-2"
      >
        <span class="min-w-0 flex-1 truncate font-mono text-xs text-mnt-secondary">{{ g.scopeId }}</span>
        <div class="hidden h-1.5 w-32 overflow-hidden rounded-full bg-mnt-surface sm:block">
          <div class="h-full rounded-full bg-mnt-green-500" :style="{ width: pct(g.progress) }" />
        </div>
        <AnomalyBadge :state="g.state" :progress="g.progress" />
      </li>
    </ul>
  </div>
</template>
