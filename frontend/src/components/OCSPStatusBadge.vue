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
import type { OCSPStatus } from '@/services/certificateApi'

defineProps<{
  status: OCSPStatus
}>()

const styles: Record<OCSPStatus, { bg: string; color: string; label: string }> = {
  good: {
    bg: 'var(--pb-status-ok-bg)',
    color: 'var(--pb-status-ok)',
    label: 'Good',
  },
  revoked: {
    bg: 'var(--pb-status-down-bg)',
    color: 'var(--pb-status-down)',
    label: 'Revoked',
  },
  unknown: {
    bg: 'var(--pb-bg-elevated)',
    color: 'var(--pb-text-muted)',
    label: 'Unknown',
  },
  error: {
    bg: 'var(--pb-status-warn-bg)',
    color: 'var(--pb-status-warn)',
    label: 'Parse error',
  },
}
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
    :style="{
      backgroundColor: styles[status].bg,
      color: styles[status].color,
    }"
  >
    OCSP {{ styles[status].label }}
  </span>
</template>
