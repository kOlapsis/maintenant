<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { computed } from 'vue'
import { ShieldAlert } from 'lucide-vue-next'

const props = defineProps<{
  count: number
  severity: string | null
}>()

const severityStyle = computed(() => {
  switch (props.severity) {
    case 'critical':
      return { color: 'var(--pb-status-down)', bg: 'var(--pb-status-down-bg)' }
    case 'high':
      return { color: 'var(--pb-status-warn)', bg: 'var(--pb-status-warn-bg)' }
    case 'medium':
      return { color: '#3b82f6', bg: 'rgba(59,130,246,0.1)' }
    default:
      return { color: 'var(--pb-text-muted)', bg: 'var(--pb-bg-elevated)' }
  }
})
</script>

<template>
  <div
    v-if="count > 0"
    class="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium"
    :style="{ color: severityStyle.color, backgroundColor: severityStyle.bg }"
    :title="`${count} security insight(s)`"
  >
    <ShieldAlert :size="9" />
    <span>{{ count }}</span>
  </div>
</template>
