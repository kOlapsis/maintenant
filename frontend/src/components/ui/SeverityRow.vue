<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { ChevronRight, Server } from 'lucide-vue-next'
import StatusDot from './StatusDot.vue'
import type { Severity } from '@/composables/useSeverity'

const props = withDefaults(
  defineProps<{
    severity: Severity
    name: string
    kind?: string
    description?: string
    /** Reporting host, shown on multi-host installs so identically-named rows stay distinguishable. */
    host?: string
    timestamp?: string
    metric?: string
    interactive?: boolean
  }>(),
  { interactive: true },
)

const emit = defineEmits<{ select: [] }>()

function activate() {
  if (props.interactive) emit('select')
}
function onKey(e: KeyboardEvent) {
  if (props.interactive && (e.key === 'Enter' || e.key === ' ')) {
    e.preventDefault()
    emit('select')
  }
}
</script>

<template>
  <div
    class="mnt-row flex items-center gap-3"
    :class="interactive ? 'focus-ring cursor-pointer' : ''"
    :role="interactive ? 'button' : undefined"
    :tabindex="interactive ? 0 : undefined"
    @click="activate"
    @keydown="onKey"
  >
    <StatusDot :severity="severity" size="md" :pulse="severity === 'incident'" />
    <span class="shrink-0 truncate text-sm font-semibold text-mnt-primary mnt-row-name">{{ name }}</span>
    <span
      v-if="kind"
      class="shrink-0 rounded border border-mnt-default px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-mnt-muted"
    >
      {{ kind }}
    </span>
    <span
      v-if="host"
      class="flex shrink-0 items-center gap-1 text-[11px] text-mnt-muted"
      :title="`Host: ${host}`"
    >
      <Server :size="11" aria-hidden="true" />
      <span class="max-w-[9rem] truncate">{{ host }}</span>
    </span>
    <span v-if="description" class="min-w-0 flex-1 truncate text-xs text-mnt-muted">{{ description }}</span>
    <span v-else class="flex-1" />
    <span v-if="metric" class="shrink-0 font-mono text-xs text-mnt-secondary">{{ metric }}</span>
    <span v-if="timestamp" class="shrink-0 text-right font-mono text-[11px] text-mnt-muted mnt-row-when">{{ timestamp }}</span>
    <slot name="actions" />
    <ChevronRight v-if="interactive" :size="15" class="shrink-0 text-mnt-muted" aria-hidden="true" />
  </div>
</template>

<style scoped>
.mnt-row {
  padding: var(--mnt-density-row-padding) 0.75rem;
  border-radius: var(--mnt-radius-md);
  transition: background-color 0.12s;
}
.mnt-row:hover {
  background: var(--mnt-bg-hover);
}
.mnt-row-name {
  min-width: 0;
  max-width: 11rem;
}
.mnt-row-when {
  min-width: 3.5rem;
}
</style>
