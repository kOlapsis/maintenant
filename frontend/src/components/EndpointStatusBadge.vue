<script setup lang="ts">
defineProps<{
  status: 'up' | 'down' | 'unknown'
}>()

const statusColors: Record<string, string> = {
  up: 'bg-green-100 text-green-800',
  down: 'bg-red-100 text-red-800',
  unknown: 'bg-gray-100 text-gray-600',
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
