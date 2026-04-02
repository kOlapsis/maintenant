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
  if (svc.crash_loop) return 'text-red-400'
  if (svc.running_replicas >= svc.desired_replicas) return 'text-pb-status-ok'
  if (svc.running_replicas > 0) return 'text-amber-400'
  return 'text-red-400'
}

function updateStateStyle(state: string): string {
  switch (state) {
    case 'updating': return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
    case 'paused': return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
    case 'rollback_started': case 'rollback_paused': case 'rollback_completed':
      return 'text-red-400 bg-red-400/10 border-red-400/20'
    default: return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
  }
}
</script>

<template>
  <div>
    <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-3">Services</p>

    <div v-if="services.length === 0" class="text-sm text-slate-500 py-4 text-center">
      No services
    </div>

    <div v-else class="space-y-1">
      <div
        v-for="svc in sortedServices"
        :key="svc.service_id"
        class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3 hover:bg-slate-800/25 transition-all"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3 min-w-0">
            <span class="text-sm text-pb-primary font-medium truncate">{{ svc.name }}</span>
            <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
              {{ svc.mode }}
            </span>
            <span
              v-if="svc.crash_loop"
              class="text-[10px] font-bold uppercase tracking-wider text-red-400 bg-red-400/10 border border-red-400/20 px-1.5 py-0.5 rounded animate-pulse"
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
