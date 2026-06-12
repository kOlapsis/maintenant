<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'grid' | 'list' | 'cards' | 'lines'
    count?: number
  }>(),
  { variant: 'lines', count: 6 },
)
</script>

<template>
  <div role="status" aria-busy="true" aria-live="polite" class="w-full">
    <span class="sr-only">Loading…</span>
    <div
      v-if="variant === 'grid'"
      class="grid gap-2"
      style="grid-template-columns: repeat(auto-fill, minmax(118px, 1fr))"
    >
      <div v-for="i in count" :key="i" class="pb-skeleton h-14 rounded-lg" />
    </div>
    <div v-else-if="variant === 'cards'" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="i in count" :key="i" class="pb-skeleton h-28 rounded-xl" />
    </div>
    <div v-else class="space-y-2">
      <div
        v-for="i in count"
        :key="i"
        class="pb-skeleton rounded-lg"
        :class="variant === 'list' ? 'h-12' : 'h-5'"
      />
    </div>
  </div>
</template>
