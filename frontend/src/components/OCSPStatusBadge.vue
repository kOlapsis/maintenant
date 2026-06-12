<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import type { Severity } from '@/composables/useSeverity'
import type { OCSPStatus } from '@/services/certificateApi'

const props = defineProps<{
  status: OCSPStatus
}>()

const map: Record<OCSPStatus, { severity: Severity; label: string }> = {
  good: { severity: 'ok', label: 'OCSP Good' },
  revoked: { severity: 'incident', label: 'OCSP Revoked' },
  unknown: { severity: 'unknown', label: 'OCSP Unknown' },
  error: { severity: 'warning', label: 'OCSP Parse error' },
}
const cfg = computed(() => map[props.status])
</script>

<template>
  <StatusBadge :severity="cfg.severity" :label="cfg.label" show-label size="sm" />
</template>
