<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
      return { color: 'var(--mnt-status-down)', bg: 'var(--mnt-status-down-bg)' }
    case 'high':
      return { color: 'var(--mnt-status-warn)', bg: 'var(--mnt-status-warn-bg)' }
    case 'medium':
      return { color: '#3b82f6', bg: 'rgba(59,130,246,0.1)' }
    default:
      return { color: 'var(--mnt-text-muted)', bg: 'var(--mnt-bg-elevated)' }
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
