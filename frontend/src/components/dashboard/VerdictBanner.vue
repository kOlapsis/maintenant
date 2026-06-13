<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { AlertTriangle, CheckCircle2 } from 'lucide-vue-next'
import { severityVar, type Severity } from '@/composables/useSeverity'

const props = defineProps<{
  incidents: number
  warnings: number
  ok: number
  summary?: string
}>()

const variant = computed<'incident' | 'warning' | 'operational'>(() =>
  props.incidents > 0 ? 'incident' : props.warnings > 0 ? 'warning' : 'operational',
)

const severity = computed<Severity>(() =>
  variant.value === 'operational' ? 'ok' : variant.value === 'warning' ? 'warning' : 'incident',
)

const title = computed(() => {
  if (variant.value === 'incident') {
    return `${props.incidents} incident${props.incidents > 1 ? 's' : ''} need${props.incidents > 1 ? '' : 's'} your attention`
  }
  if (variant.value === 'warning') {
    return `${props.warnings} warning${props.warnings > 1 ? 's' : ''} to review`
  }
  return 'All systems operational'
})

const counters = computed(() => [
  { label: 'Incident', value: props.incidents, color: severityVar('incident', 'text') },
  { label: 'Warning', value: props.warnings, color: severityVar('warning', 'text') },
  { label: 'OK', value: props.ok, color: severityVar('ok', 'text') },
])

const boxStyle = computed(() => ({
  borderColor: severityVar(severity.value, 'border'),
  background: severityVar(severity.value, 'bg'),
}))
</script>

<template>
  <div
    class="flex items-center gap-4 rounded-2xl border p-4 sm:p-5"
    :style="boxStyle"
    role="status"
    aria-live="polite"
  >
    <span
      class="relative grid h-11 w-11 shrink-0 place-items-center rounded-xl"
      :style="{ backgroundColor: severityVar(severity, 'bg'), color: severityVar(severity, 'text') }"
    >
      <span
        v-if="variant === 'incident'"
        class="mnt-ping absolute inset-0 rounded-xl"
        :style="{ backgroundColor: severityVar('incident') }"
        aria-hidden="true"
      />
      <component
        :is="variant === 'operational' ? CheckCircle2 : AlertTriangle"
        :size="22"
        class="relative"
        aria-hidden="true"
      />
    </span>

    <div class="min-w-0">
      <h1 class="text-base font-semibold tracking-tight text-mnt-primary sm:text-lg">{{ title }}</h1>
      <p v-if="summary" class="mt-0.5 truncate text-xs text-mnt-muted sm:text-sm">{{ summary }}</p>
    </div>

    <div class="ml-auto flex shrink-0 gap-2 sm:gap-2.5">
      <div
        v-for="c in counters"
        :key="c.label"
        class="flex min-w-[58px] flex-col items-center rounded-xl border border-mnt-default bg-mnt-surface px-3 py-2"
      >
        <span class="text-lg font-bold leading-none sm:text-xl" :style="{ color: c.color }">{{ c.value }}</span>
        <span class="mt-1 text-[10px] uppercase tracking-wide text-mnt-muted">{{ c.label }}</span>
      </div>
    </div>
  </div>
</template>
