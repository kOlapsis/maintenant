<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
    class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer rounded-full bg-mnt-green-600 px-3 py-1 text-sm text-mnt-inverted shadow-lg transition-all hover:bg-mnt-green-500"
    @click="$emit('click')"
  >
    <ArrowDown :size="12" class="mr-1 inline-block" />
    {{ unseenCount }} new line{{ unseenCount === 1 ? '' : 's' }}
  </div>
</template>
