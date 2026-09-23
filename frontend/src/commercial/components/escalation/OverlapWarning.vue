<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next'
import type { OverlapWarning } from '@/commercial/types/escalation'

defineProps<{
  warnings: OverlapWarning[]
}>()
</script>

<template>
  <div v-if="warnings.length > 0" class="space-y-2">
    <p class="text-[10px] text-mnt-status-warn font-bold uppercase tracking-widest flex items-center gap-1.5">
      <AlertTriangle :size="11" />
      Overlap warnings
    </p>
    <div
      v-for="w in warnings"
      :key="w.policy_id"
      class="px-3 py-2 rounded-lg bg-mnt-sev-warning-solid/5 border border-amber-500/20 text-xs text-mnt-status-warn"
    >
      Conflict with <span class="font-semibold">"{{ w.policy_name }}"</span>
      — shared channels: {{ w.shared_channels.join(', ') }}
    </div>
  </div>
</template>
