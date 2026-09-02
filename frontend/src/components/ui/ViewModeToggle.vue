<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { LayoutGrid, Rows3, Table2 } from 'lucide-vue-next'
import SegmentedToggle from './SegmentedToggle.vue'
import { usePreferencesStore } from '@/stores/preferences'
import type { ListScope, ListView } from '@/stores/preferences'

const props = defineProps<{ scope: ListScope }>()

const prefs = usePreferencesStore()

const options = [
  { value: 'cards', icon: LayoutGrid, title: 'Cards' },
  { value: 'rows', icon: Rows3, title: 'Rows' },
  { value: 'table', icon: Table2, title: 'Table' },
]

const model = computed<string>({
  get: () => prefs.listView(props.scope),
  set: (value) => prefs.setListView(props.scope, value as ListView),
})
</script>

<template>
  <SegmentedToggle v-model="model" :options="options" ariaLabel="View" />
</template>
