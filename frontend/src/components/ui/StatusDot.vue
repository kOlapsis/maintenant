<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { severityVar, severityMeta, type Severity } from '@/composables/useSeverity'

const props = withDefaults(
  defineProps<{
    severity: Severity
    size?: 'sm' | 'md' | 'lg'
    pulse?: boolean
    label?: string
    /** When inside a labelled parent (e.g. StatusBadge), render purely decorative. */
    decorative?: boolean
  }>(),
  { size: 'md', pulse: false, decorative: false },
)

const sizeClass = { sm: 'w-2 h-2', md: 'w-2.5 h-2.5', lg: 'w-3 h-3' }
const ariaLabel = computed(() => props.label ?? severityMeta(props.severity).label)
</script>

<template>
  <span
    class="relative inline-flex shrink-0"
    :class="sizeClass[size]"
    :role="decorative ? undefined : 'img'"
    :aria-label="decorative ? undefined : ariaLabel"
    :aria-hidden="decorative ? 'true' : undefined"
  >
    <span
      v-if="pulse"
      class="pb-ping absolute inset-0 rounded-full"
      :style="{ backgroundColor: severityVar(severity) }"
      aria-hidden="true"
    />
    <span
      class="relative inline-flex h-full w-full rounded-full"
      :style="{ backgroundColor: severityVar(severity) }"
    />
  </span>
</template>
