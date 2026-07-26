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
  status: 'up' | 'down' | 'degraded' | 'unknown'
}>()

const map: Record<'up' | 'down' | 'degraded' | 'unknown', { severity: Severity; label: string }> = {
  up: { severity: 'ok', label: 'Up' },
  down: { severity: 'incident', label: 'Down' },
  // Reachable, but over a certificate we do not trust — a warning, not an outage.
  degraded: { severity: 'warning', label: 'Degraded' },
  unknown: { severity: 'unknown', label: 'Unknown' },
}
const cfg = computed(() => map[props.status])
</script>

<template>
  <StatusBadge :severity="cfg.severity" :label="cfg.label" show-label size="sm" />
</template>
