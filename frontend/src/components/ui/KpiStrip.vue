<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { type Component } from 'vue'
import type { RouteLocationRaw } from 'vue-router'
import KpiStat from './KpiStat.vue'
import type { Severity } from '@/composables/useSeverity'

export interface KpiStripItem {
  label: string
  value: string
  sub?: string
  icon?: Component
  tone?: Severity
  to?: RouteLocationRaw
}

const props = defineProps<{ stats: KpiStripItem[] }>()
</script>

<template>
  <div
    class="pb-kpis grid overflow-hidden rounded-xl border border-pb-default"
    :style="{ '--kpi-cols': props.stats.length }"
  >
    <KpiStat v-for="(s, i) in stats" :key="i" v-bind="s" class="bg-pb-surface" />
  </div>
</template>

<style scoped>
.pb-kpis {
  gap: 1px;
  background: var(--pb-border-default);
  grid-template-columns: repeat(2, 1fr);
}
@media (min-width: 1024px) {
  .pb-kpis {
    grid-template-columns: repeat(var(--kpi-cols), minmax(0, 1fr));
  }
}
</style>
