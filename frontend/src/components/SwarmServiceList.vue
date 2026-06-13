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
import type { SwarmDashboardService } from '@/services/swarmApi'

const props = defineProps<{
  services: SwarmDashboardService[]
}>()

const sortedServices = computed(() => {
  return [...props.services].sort((a, b) => a.name.localeCompare(b.name))
})

function replicaColor(svc: SwarmDashboardService): string {
  if (svc.crash_loop) return 'text-pb-status-down'
  if (svc.running_replicas >= svc.desired_replicas) return 'text-pb-status-ok'
  if (svc.running_replicas > 0) return 'text-pb-status-warn'
  return 'text-pb-status-down'
}

function updateStateStyle(state: string): string {
  switch (state) {
    case 'updating': return 'text-pb-secondary bg-sky-400/10 border-sky-400/20'
    case 'paused': return 'text-pb-status-warn bg-pb-status-warn border-pb-sev-warning'
    case 'rollback_started': case 'rollback_paused': case 'rollback_completed':
      return 'text-pb-status-down bg-pb-status-down border-pb-sev-incident'
    default: return 'text-pb-muted bg-pb-elevated border-pb-default'
  }
}
</script>

<template>
  <div>
    <p class="text-[10px] text-pb-muted font-bold uppercase tracking-widest mb-3">Services</p>

    <div v-if="services.length === 0" class="text-sm text-pb-muted py-4 text-center">
      No services
    </div>

    <div v-else class="space-y-1">
      <div
        v-for="svc in sortedServices"
        :key="svc.service_id"
        class="bg-pb-surface rounded-xl border border-pb-default px-4 py-3 hover:bg-pb-elevated transition-all"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-sm text-pb-primary font-medium truncate">{{ svc.name }}</span>
            <span class="text-[10px] font-bold uppercase tracking-wider text-pb-muted bg-pb-elevated border border-pb-default px-1.5 py-0.5 rounded">
              {{ svc.mode }}
            </span>
            <span
              v-if="svc.crash_loop"
              class="text-[10px] font-bold uppercase tracking-wider text-pb-status-down bg-pb-status-down border border-pb-sev-incident px-1.5 py-0.5 rounded animate-pulse"
            >
              crash loop
            </span>
            <span
              v-if="svc.update_state"
              :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', updateStateStyle(svc.update_state)]"
            >
              {{ svc.update_state.replace(/_/g, ' ') }}
            </span>
          </div>

          <div class="flex items-center gap-2 flex-shrink-0 ml-4">
            <span :class="['text-sm font-medium tabular-nums', replicaColor(svc)]">
              {{ svc.running_replicas }}/{{ svc.desired_replicas }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
