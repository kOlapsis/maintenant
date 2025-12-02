<script setup lang="ts">
import { usePreferencesStore } from '@/stores/preferences'

const prefs = usePreferencesStore()
</script>

<template>
  <button
    type="button"
    @click="prefs.toggleDensity()"
    class="inline-flex items-center justify-center rounded-md p-1.5 transition-colors hover:bg-[var(--pb-bg-hover)]"
    :aria-label="prefs.density === 'comfortable' ? 'Switch to compact view' : 'Switch to comfortable view'"
  >
    <!-- Compact icon (narrow lines) shown when currently comfortable -->
    <svg
      v-if="prefs.density === 'comfortable'"
      width="18"
      height="18"
      viewBox="0 0 18 18"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
    >
      <line x1="3" y1="5" x2="15" y2="5" />
      <line x1="3" y1="8" x2="15" y2="8" />
      <line x1="3" y1="11" x2="15" y2="11" />
      <line x1="3" y1="14" x2="15" y2="14" />
    </svg>
    <!-- Comfortable icon (spaced lines) shown when currently compact -->
    <svg
      v-else
      width="18"
      height="18"
      viewBox="0 0 18 18"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
    >
      <line x1="3" y1="4" x2="15" y2="4" />
      <line x1="3" y1="9" x2="15" y2="9" />
      <line x1="3" y1="14" x2="15" y2="14" />
    </svg>
  </button>
</template>
