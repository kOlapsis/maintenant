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
import type { MonitorRef } from '@/services/statusApi'

defineProps<{
  monitors: MonitorRef[]
}>()

const statusColors: Record<string, string> = {
  operational: 'var(--pb-status-ok)',
  degraded: 'var(--pb-status-warn)',
  partial_outage: 'var(--pb-status-critical)',
  major_outage: 'var(--pb-status-down)',
  under_maintenance: 'var(--pb-accent)',
}

const typeLabels: Record<string, string> = {
  container: 'Container',
  endpoint: 'Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'Certificate',
}
</script>

<template>
  <div class="mt-2 space-y-1 rounded-lg border border-slate-800 bg-[#0B0E13] p-3">
    <div v-if="!monitors?.length" class="text-xs text-slate-500">No monitors</div>
    <div
      v-for="m in monitors"
      :key="`${m.type}-${m.id}`"
      class="flex items-center gap-3 rounded px-2 py-1.5"
    >
      <span
        class="h-2 w-2 flex-shrink-0 rounded-full"
        :style="{ background: statusColors[m.status ?? 'operational'] || 'var(--pb-text-muted)' }"
      ></span>
      <span class="flex-1 text-sm text-slate-200">{{ m.name || `${typeLabels[m.type] || m.type} #${m.id}` }}</span>
      <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">{{ typeLabels[m.type] || m.type }}</span>
    </div>
  </div>
</template>
