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
import { computed } from 'vue'
import type { Container } from '@/services/containerApi'
import { useResourcesStore } from '@/stores/resources'
import { useUpdatesStore } from '@/stores/updates'
import { timeAgo } from '@/utils/time'
import { getStateStyle } from '@/utils/containerState'
import UpdateBadge from '@/components/UpdateBadge.vue'
import SecurityInsightBadge from '@/components/SecurityInsightBadge.vue'
import ListRow from '@/components/ui/ListRow.vue'
import type { ChipTone } from '@/components/ui/listFilters'

const props = defineProps<{
  container: Container
  tone: ChipTone
  /** Compose project or namespace, shown as context now that rows are flat. */
  group?: string
}>()

const emit = defineEmits<{
  select: [container: Container]
}>()

const resourcesStore = useResourcesStore()
const updatesStore = useUpdatesStore()

const metrics = computed(() => resourcesStore.formattedSnapshot(props.container.id))
const containerUpdate = computed(
  () => updatesStore.updates.find((u) => u.container_id === props.container.external_id) ?? null,
)

const healthColors: Record<string, string> = {
  healthy: 'var(--mnt-status-ok)',
  unhealthy: 'var(--mnt-status-down)',
  starting: 'var(--mnt-status-warn)',
}

const cpuBarWidth = computed(() => {
  const snap = resourcesStore.getSnapshot(props.container.id)
  if (!snap) return 0
  return Math.min(snap.cpu_percent, 100)
})

const memBarWidth = computed(() => {
  const snap = resourcesStore.getSnapshot(props.container.id)
  if (!snap || snap.mem_limit === 0) return 0
  return Math.min((snap.mem_used / snap.mem_limit) * 100, 100)
})

function barColor(value: number): string {
  if (value > 80) return 'var(--mnt-status-down)'
  if (value > 50) return 'var(--mnt-status-warn)'
  return 'var(--mnt-status-ok)'
}

const imageTag = computed(() => {
  const base = props.container.image.split('@')[0] ?? props.container.image
  const parts = base.split(':')
  return parts.length > 1 ? parts[parts.length - 1] : base
})

const stateStyle = computed(() => {
  const s = getStateStyle(props.container.state)
  return { backgroundColor: s.bg, color: s.color }
})

const showMetrics = computed(() => props.container.state === 'running' && metrics.value !== null)
</script>

<template>
  <ListRow :tone="tone" @select="emit('select', container)">
    <!-- Fixed columns, so names of wildly different lengths still line up. -->
    <div class="row-grid w-full items-center gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-1.5">
          <span
            v-if="container.has_health_check && container.health_status"
            class="inline-block h-2 w-2 shrink-0 rounded-full"
            :style="{
              backgroundColor:
                container.state === 'running'
                  ? (healthColors[container.health_status] || 'var(--mnt-text-muted)')
                  : 'var(--mnt-text-muted)',
            }"
            :title="container.state === 'running' ? container.health_status : 'stopped'"
          />
          <span class="truncate text-sm font-semibold text-mnt-primary">{{ container.name }}</span>
        </div>
        <p class="truncate text-[11px] text-mnt-muted">
          <span v-if="group">{{ group }} &middot; </span>{{ imageTag }}
        </p>
      </div>

      <div class="col-badges flex items-center justify-end gap-1.5">
        <UpdateBadge v-if="containerUpdate" :update="containerUpdate" />
        <SecurityInsightBadge
          :count="container.security_insight_count ?? 0"
          :severity="container.security_highest_severity ?? null"
        />
      </div>

      <div>
        <span
          v-if="container.stale"
          class="inline-flex items-center rounded-full bg-mnt-sev-unknown px-2 py-0.5 text-[10px] font-bold text-mnt-sev-unknown"
          :title="`Agent offline, last known: ${container.state}`"
        >offline</span>
        <span
          v-else
          class="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold"
          :style="stateStyle"
        >{{ container.state }}</span>
      </div>

      <div class="col-metric flex items-center gap-1.5 text-[10px]">
        <template v-if="showMetrics && metrics">
          <span class="font-bold uppercase text-mnt-muted">CPU</span>
          <div class="h-1 flex-1 rounded-full bg-mnt-primary">
            <div
              class="h-1 rounded-full"
              :style="{ width: cpuBarWidth + '%', backgroundColor: barColor(cpuBarWidth) }"
            />
          </div>
          <span class="w-10 text-right font-mono text-mnt-muted">{{ metrics.cpu }}</span>
        </template>
      </div>

      <div class="col-metric flex items-center gap-1.5 text-[10px]">
        <template v-if="showMetrics && metrics">
          <span class="font-bold uppercase text-mnt-muted">MEM</span>
          <div class="h-1 flex-1 rounded-full bg-mnt-primary">
            <div
              class="h-1 rounded-full"
              :style="{ width: memBarWidth + '%', backgroundColor: barColor(memBarWidth) }"
            />
          </div>
          <span class="w-10 text-right font-mono text-mnt-muted">{{ metrics.memPercent }}</span>
        </template>
      </div>

      <div class="text-right text-[11px] text-mnt-muted">
        {{ timeAgo(container.last_state_change_at) }}
      </div>
    </div>
  </ListRow>
</template>

<style scoped>
.row-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 92px 64px;
}
.col-badges,
.col-metric {
  display: none;
}

@media (min-width: 640px) {
  .row-grid {
    grid-template-columns: minmax(0, 1fr) auto 92px 64px;
  }
  .col-badges {
    display: flex;
  }
}

@media (min-width: 1024px) {
  .row-grid {
    grid-template-columns: minmax(0, 1fr) auto 92px 148px 148px 64px;
  }
  .col-metric {
    display: flex;
  }
}
</style>
