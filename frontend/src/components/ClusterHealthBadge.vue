<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { Heart, AlertTriangle, XCircle } from 'lucide-vue-next'

const props = defineProps<{
  health: 'healthy' | 'degraded' | 'unhealthy'
}>()

const config = computed(() => {
  switch (props.health) {
    case 'healthy':
      return {
        icon: Heart,
        label: 'Healthy',
        style: 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20',
      }
    case 'degraded':
      return {
        icon: AlertTriangle,
        label: 'Degraded',
        style: 'text-amber-400 bg-amber-400/10 border-amber-400/20',
      }
    case 'unhealthy':
      return {
        icon: XCircle,
        label: 'Unhealthy',
        style: 'text-red-400 bg-red-400/10 border-red-400/20',
      }
  }
})
</script>

<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold',
      config.style,
    ]"
  >
    <component :is="config.icon" class="h-3 w-3" />
    {{ config.label }}
  </span>
</template>
