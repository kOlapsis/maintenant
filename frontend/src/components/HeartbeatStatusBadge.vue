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
defineProps<{
  status: 'new' | 'up' | 'down' | 'started' | 'paused'
}>()

const statusColors: Record<string, string> = {
  new: 'bg-gray-100 text-slate-700',
  up: 'bg-green-100 text-green-800',
  down: 'bg-red-100 text-red-800',
  started: 'bg-pb-green-500/15 text-pb-green-700',
  paused: 'bg-yellow-100 text-yellow-800',
}
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
    :class="statusColors[status] || 'bg-gray-100 text-slate-700'"
  >
    {{ status }}
  </span>
</template>
