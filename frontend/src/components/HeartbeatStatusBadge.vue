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
  status: 'new' | 'up' | 'down' | 'started' | 'paused'
}>()

const map: Record<'new' | 'up' | 'down' | 'started' | 'paused', { severity: Severity; label: string }> = {
  new: { severity: 'unknown', label: 'New' },
  up: { severity: 'ok', label: 'Up' },
  down: { severity: 'incident', label: 'Down' },
  started: { severity: 'ok', label: 'Started' },
  paused: { severity: 'neutral', label: 'Paused' },
}
const cfg = computed(() => map[props.status])
</script>

<template>
  <StatusBadge :severity="cfg.severity" :label="cfg.label" show-label size="sm" />
</template>
