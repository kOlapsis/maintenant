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
  status: 'up' | 'down' | 'unknown'
}>()

const statusColors: Record<string, string> = {
  up: 'bg-green-100 text-green-800',
  down: 'bg-red-100 text-red-800',
  unknown: 'bg-gray-100 text-slate-700',
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
