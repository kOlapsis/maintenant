<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import StatusDot from './StatusDot.vue'
import { severityMeta, severityVar, type Severity } from '@/composables/useSeverity'

// Legacy status union, kept so existing callers (`:status="..."`) keep working.
// `critical` and `down` both collapse to the single `incident` severity.
export type BadgeStatus = 'ok' | 'warning' | 'critical' | 'down' | 'paused' | 'unknown'

const props = withDefaults(
  defineProps<{
    severity?: Severity
    status?: BadgeStatus
    size?: 'sm' | 'md' | 'lg'
    label?: string
    showLabel?: boolean
    showIcon?: boolean
    pulse?: boolean
  }>(),
  { size: 'md', showLabel: false, showIcon: true, pulse: false },
)

const LEGACY_TO_SEVERITY: Record<BadgeStatus, Severity> = {
  ok: 'ok',
  warning: 'warning',
  critical: 'incident',
  down: 'incident',
  paused: 'neutral',
  unknown: 'unknown',
}

const sev = computed<Severity>(() =>
  props.severity ?? (props.status ? LEGACY_TO_SEVERITY[props.status] : 'unknown'),
)
const meta = computed(() => severityMeta(sev.value))
const label = computed(() => props.label ?? meta.value.label)

const iconSizes = { sm: 10, md: 14, lg: 18 }
const textSizes = { sm: 'text-xs', md: 'text-sm', lg: 'text-base' }
</script>

<template>
  <span class="inline-flex items-center gap-1.5" :style="{ color: severityVar(sev, 'text') }">
    <StatusDot :severity="sev" :size="size" :pulse="pulse" decorative />
    <component :is="meta.icon" v-if="showIcon" :size="iconSizes[size]" aria-hidden="true" />
    <span v-if="showLabel" :class="textSizes[size]">{{ label }}</span>
    <span v-else class="sr-only">{{ label }}</span>
  </span>
</template>
