<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { severityVar, severityMeta, type Severity } from '@/composables/useSeverity'

const props = defineProps<{
  severity: Severity
  name: string
  meta?: string
}>()

const emit = defineEmits<{ select: [] }>()

// Non-healthy tiles get a muted wash + corner marker so severity reads without
// relying on colour alone (the left rail + marker are the secondary channels).
const tintClass = computed(() =>
  props.severity === 'incident'
    ? 'bg-mnt-sev-incident'
    : props.severity === 'warning'
      ? 'bg-mnt-sev-warning'
      : '',
)
const marked = computed(() => props.severity === 'incident' || props.severity === 'warning')

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    emit('select')
  }
}
</script>

<template>
  <div
    class="mnt-tile focus-ring"
    :class="tintClass"
    role="button"
    tabindex="0"
    :style="{ borderLeftColor: severityVar(severity) }"
    @click="emit('select')"
    @keydown="onKey"
  >
    <div class="truncate text-[11.5px] font-semibold text-mnt-primary">{{ name }}</div>
    <div v-if="meta" class="mt-0.5 truncate font-mono text-[10px] text-mnt-muted">{{ meta }}</div>
    <span
      v-if="marked"
      class="mnt-tile-mark"
      :style="{ backgroundColor: severityVar(severity) }"
      aria-hidden="true"
    />
    <span class="sr-only">{{ severityMeta(severity).label }}</span>
  </div>
</template>

<style scoped>
.mnt-tile {
  position: relative;
  cursor: pointer;
  border: 1px solid var(--mnt-border-default);
  border-left-width: 3px;
  border-radius: var(--mnt-radius-md);
  background: var(--mnt-bg-elevated);
  padding: 9px 10px;
  transition:
    transform 0.08s,
    border-color 0.12s;
}
.mnt-tile:hover {
  transform: translateY(-1px);
}
.mnt-tile-mark {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
@media (prefers-reduced-motion: reduce) {
  .mnt-tile {
    transition: none;
  }
  .mnt-tile:hover {
    transform: none;
  }
}
</style>
