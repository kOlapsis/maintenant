<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import type { Severity } from '@/composables/useSeverity'
import type { CertStatus } from '@/services/certificateApi'

const props = defineProps<{
  status: CertStatus
}>()

const map: Record<CertStatus, { severity: Severity; label: string }> = {
  valid: { severity: 'ok', label: 'Valid' },
  expiring: { severity: 'warning', label: 'Expiring' },
  expired: { severity: 'incident', label: 'Expired' },
  error: { severity: 'incident', label: 'Error' },
  unknown: { severity: 'unknown', label: 'Unknown' },
}
const cfg = computed(() => map[props.status])
</script>

<template>
  <StatusBadge :severity="cfg.severity" :label="cfg.label" show-label size="sm" />
</template>
