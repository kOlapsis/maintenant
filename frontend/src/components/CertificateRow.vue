<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { CertMonitor } from '@/services/certificateApi'
import {
  certificateTone,
  countdownColor,
  formatDaysRemaining,
  formatExpiryDate,
} from '@/utils/certFormat'
import ListRow from '@/components/ui/ListRow.vue'
import CertificateStatusBadge from './CertificateStatusBadge.vue'

const props = defineProps<{ certificate: CertMonitor }>()

const emit = defineEmits<{ select: [id: string] }>()

const days = computed(() => props.certificate.latest_check?.days_remaining)
const issuer = computed(() => props.certificate.latest_check?.issuer_cn || 'Unknown issuer')
</script>

<template>
  <ListRow
    :tone="certificateTone(certificate.status)"
    :aria-label="`${certificate.hostname}, ${certificate.status}, ${formatDaysRemaining(days)} left`"
    @select="emit('select', certificate.id)"
  >
    <!-- Fixed columns, so status badges of different widths do not shift the countdown. -->
    <div class="row-grid w-full items-center gap-3">
      <div class="min-w-0">
        <p class="truncate text-sm font-medium text-mnt-primary">
          {{ certificate.hostname }}<span class="text-mnt-muted">:{{ certificate.port }}</span>
        </p>
        <p class="truncate text-xs text-mnt-muted">{{ issuer }}</p>
      </div>

      <div>
        <CertificateStatusBadge :status="certificate.status" />
      </div>

      <span
        class="text-right font-mono text-sm font-semibold tabular-nums"
        :style="{ color: countdownColor(days) }"
      >
        {{ formatDaysRemaining(days) }}
      </span>

      <span class="col-expiry text-right text-xs text-mnt-muted">
        {{ formatExpiryDate(certificate.latest_check?.not_after) }}
      </span>
    </div>
  </ListRow>
</template>

<style scoped>
.row-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 88px 80px;
}
.col-expiry {
  display: none;
}

@media (min-width: 640px) {
  .row-grid {
    grid-template-columns: minmax(0, 1fr) 88px 80px 112px;
  }
  .col-expiry {
    display: block;
  }
}
</style>
