<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  title: string
  count?: number | string
  icon?: Component
}>()
</script>

<template>
  <div class="flex items-center gap-2.5">
    <component :is="icon" v-if="icon" :size="15" class="text-mnt-muted" aria-hidden="true" />
    <h2 class="text-xs font-semibold uppercase tracking-wide text-mnt-muted">{{ title }}</h2>
    <span
      v-if="count !== undefined"
      class="rounded-full bg-mnt-elevated px-2 py-0.5 font-mono text-[11px] text-mnt-muted"
    >
      {{ count }}
    </span>
    <div class="ml-auto">
      <slot name="action" />
    </div>
  </div>
</template>
