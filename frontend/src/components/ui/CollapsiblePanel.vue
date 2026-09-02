<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed, useId } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import { usePreferencesStore } from '@/stores/preferences'

const props = defineProps<{
  /** Persistence key, stored as mnt-panel:<storageKey>. */
  storageKey: string
  title: string
}>()

const prefs = usePreferencesStore()
const contentId = useId()

const collapsed = computed(() => prefs.isPanelCollapsed(props.storageKey))
</script>

<template>
  <section
    class="mb-6 overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface shadow-mnt-card"
  >
    <div class="flex items-center gap-3 px-4 py-2.5">
      <button
        type="button"
        class="focus-ring inline-flex items-center gap-2 text-left"
        :aria-expanded="!collapsed"
        :aria-controls="contentId"
        @click="prefs.togglePanel(storageKey)"
      >
        <ChevronDown
          :size="14"
          class="shrink-0 text-mnt-muted transition-transform"
          :class="{ '-rotate-90': collapsed }"
          aria-hidden="true"
        />
        <h2 class="text-xs font-semibold uppercase tracking-wide text-mnt-secondary">
          {{ title }}
        </h2>
      </button>

      <!-- Standing in for the panel while it is put away. -->
      <div v-if="collapsed" class="ml-auto min-w-0 text-xs text-mnt-muted">
        <slot name="summary" />
      </div>
      <div v-else class="ml-auto">
        <slot name="actions" />
      </div>
    </div>

    <div v-show="!collapsed" :id="contentId" class="px-4 pb-4">
      <slot />
    </div>
  </section>
</template>
