<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { severityFromAlert, severityVar, type Severity } from '@/composables/useSeverity'
import type { SeriesState } from '@/commercial/services/anomalyApi'

const props = defineProps<{
  state: SeriesState
  progress?: number
  active?: boolean
  alertSeverity?: string
}>()

const sev = computed<Severity>(() => {
  if (props.state === 'ready' && props.active) return severityFromAlert(props.alertSeverity ?? '')
  if (props.state === 'ready') return 'ok'
  return 'neutral'
})

const label = computed(() => {
  switch (props.state) {
    case 'learning':
      return `Learning ${Math.round((props.progress ?? 0) * 100)}%`
    case 'relearning':
      return `Re-learning ${Math.round((props.progress ?? 0) * 100)}%`
    case 'disabled':
      return 'Disabled'
    default:
      return props.active ? 'Anomaly' : 'Nominal'
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium"
    :style="{
      color: severityVar(sev, 'text'),
      backgroundColor: severityVar(sev, 'bg'),
    }"
  >
    <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: severityVar(sev) }" />
    {{ label }}
  </span>
</template>
