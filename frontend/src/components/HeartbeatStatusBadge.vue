<script setup lang="ts">
defineProps<{
  status: 'new' | 'up' | 'down' | 'started' | 'paused'
}>()

const statusColors: Record<string, string> = {
  new: 'bg-gray-100 text-gray-600',
  up: 'bg-green-100 text-green-800',
  down: 'bg-red-100 text-red-800',
  started: 'bg-blue-100 text-blue-800',
  paused: 'bg-yellow-100 text-yellow-800',
}
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
    :class="statusColors[status] || 'bg-gray-100 text-gray-600'"
  >
    {{ status }}
  </span>
</template>
