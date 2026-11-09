<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->
<script setup lang="ts">
import { RouterLink } from 'vue-router'
import StatusBadge from '@/components/ui/StatusBadge.vue'
</script>

<template>
  <span class="inline-flex flex-wrap items-center gap-x-2 gap-y-0.5">
    <StatusBadge severity="neutral" label="Not monitored: Community limit" show-label size="sm" />
    <RouterLink
      :to="{ name: 'editions' }"
      class="text-xs font-medium underline transition-opacity hover:opacity-80"
      style="color: var(--mnt-accent)"
      @click.stop
    >
      See editions
    </RouterLink>
  </span>
</template>
