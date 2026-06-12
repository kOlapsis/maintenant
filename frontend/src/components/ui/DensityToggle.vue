<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { Rows2, Rows4 } from 'lucide-vue-next'
import SegmentedToggle from './SegmentedToggle.vue'
import { usePreferencesStore } from '@/stores/preferences'

const prefs = usePreferencesStore()

const options = [
  { value: 'comfortable', icon: Rows2, title: 'Comfortable rows' },
  { value: 'compact', icon: Rows4, title: 'Compact rows' },
]

const model = computed<string>({
  get: () => prefs.density,
  set: (value) => {
    if (value !== prefs.density) prefs.toggleDensity()
  },
})
</script>

<template>
  <SegmentedToggle v-model="model" :options="options" ariaLabel="Row density" />
</template>
