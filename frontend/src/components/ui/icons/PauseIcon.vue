<script setup lang="ts">
defineProps<{ size?: number }>()
</script>

<template>
  <svg
    :width="size ?? 16"
    :height="size ?? 16"
    viewBox="0 0 16 16"
    fill="currentColor"
  >
    <rect x="4" y="3" width="2.5" height="10" rx="0.5" />
    <rect x="9.5" y="3" width="2.5" height="10" rx="0.5" />
  </svg>
</template>
