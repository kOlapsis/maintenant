<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    expanded: boolean
    controlsId?: string
    chevronPosition?: 'start' | 'end'
  }>(),
  { chevronPosition: 'start' },
)

const emit = defineEmits<{ toggle: [] }>()
</script>

<template>
  <button
    type="button"
    class="focus-ring flex min-h-[44px] w-full items-center gap-2 text-left"
    :class="chevronPosition === 'end' ? 'justify-between' : ''"
    :aria-expanded="expanded"
    :aria-controls="controlsId"
    @click="emit('toggle')"
  >
    <ChevronDown
      v-if="chevronPosition === 'start'"
      :size="14"
      class="shrink-0 transition-transform"
      :class="{ '-rotate-90': !expanded }"
      aria-hidden="true"
    />
    <span class="min-w-0 flex-1"><slot /></span>
    <ChevronDown
      v-if="chevronPosition === 'end'"
      :size="14"
      class="shrink-0 transition-transform"
      :class="{ '-rotate-180': !expanded }"
      aria-hidden="true"
    />
  </button>
</template>
