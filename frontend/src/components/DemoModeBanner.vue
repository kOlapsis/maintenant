<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import AlertBanner from '@/components/ui/AlertBanner.vue'
import { useEdition } from '@/composables/useEdition'

const { isDemo } = useEdition()
</script>

<template>
  <AlertBanner v-if="isDemo" severity="info" label="DEMO MODE" class="shrink-0">
    Demo mode: read-only. You can explore everything, changes are disabled.
  </AlertBanner>
</template>
