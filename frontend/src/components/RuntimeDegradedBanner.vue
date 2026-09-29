<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next'
</script>

<template>
  <div class="rounded-lg border border-mnt-default bg-mnt-surface px-4 py-3">
    <div class="flex items-center gap-2.5">
      <AlertTriangle
        class="h-4 w-4 shrink-0"
        style="color: var(--mnt-status-warning)"
      />
      <p class="text-sm text-mnt-muted">
        La surveillance des conteneurs est indisponible. Les données affichées peuvent être obsolètes.
      </p>
    </div>
  </div>
</template>
