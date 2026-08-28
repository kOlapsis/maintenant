<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import type { Severity } from '@/composables/useSeverity'

const props = defineProps<{
  health: 'healthy' | 'degraded' | 'unhealthy'
}>()

const map: Record<'healthy' | 'degraded' | 'unhealthy', { severity: Severity; label: string }> = {
  healthy: { severity: 'ok', label: 'Healthy' },
  degraded: { severity: 'warning', label: 'Degraded' },
  unhealthy: { severity: 'incident', label: 'Unhealthy' },
}
const cfg = computed(() => map[props.health])
</script>

<template>
  <StatusBadge :severity="cfg.severity" :label="cfg.label" show-label size="sm" />
</template>
