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
import { ArrowDown } from 'lucide-vue-next'

defineProps<{
  unseenCount: number
}>()

defineEmits<{
  click: []
}>()
</script>

<template>
  <div
    v-if="unseenCount > 0"
    class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer rounded-full bg-pb-green-600 px-3 py-1 text-sm text-slate-950 shadow-lg transition-all hover:bg-pb-green-500"
    @click="$emit('click')"
  >
    <ArrowDown :size="12" class="mr-1 inline-block" />
    {{ unseenCount }} new line{{ unseenCount === 1 ? '' : 's' }}
  </div>
</template>
