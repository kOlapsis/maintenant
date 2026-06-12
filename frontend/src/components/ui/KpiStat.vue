<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed, type Component } from 'vue'
import { RouterLink, type RouteLocationRaw } from 'vue-router'
import { severityVar, type Severity } from '@/composables/useSeverity'

const props = defineProps<{
  label: string
  value: string
  sub?: string
  icon?: Component
  tone?: Severity
  to?: RouteLocationRaw
}>()

const valueStyle = computed(() =>
  props.tone ? { color: severityVar(props.tone, 'text') } : undefined,
)
</script>

<template>
  <component
    :is="to ? RouterLink : 'div'"
    :to="to"
    class="flex flex-col gap-1 px-4 py-3.5"
    :class="to ? 'focus-ring transition-colors hover:bg-pb-elevated' : ''"
  >
    <div class="flex items-center gap-2">
      <component :is="icon" v-if="icon" :size="13" class="text-pb-muted" aria-hidden="true" />
      <span class="text-[10px] font-semibold uppercase tracking-wider text-pb-muted">{{ label }}</span>
    </div>
    <span class="text-xl font-bold text-pb-primary" :style="valueStyle">{{ value }}</span>
    <span v-if="sub" class="truncate text-[11px] text-pb-muted">{{ sub }}</span>
  </component>
</template>
