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
import { AlertTriangle } from 'lucide-vue-next'
import type { OverlapWarning } from '@/types/escalation'

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
