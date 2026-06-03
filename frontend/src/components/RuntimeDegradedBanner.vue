<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next'
</script>

<template>
  <div class="rounded-lg border border-pb-default bg-pb-surface px-4 py-3">
    <div class="flex items-center gap-2.5">
      <AlertTriangle
        class="h-4 w-4 shrink-0"
        style="color: var(--pb-status-warning)"
      />
      <p class="text-sm text-pb-muted">
        La surveillance des conteneurs est indisponible. Les données affichées peuvent être obsolètes.
      </p>
    </div>
  </div>
</template>
