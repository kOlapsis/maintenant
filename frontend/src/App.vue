<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<template>
  <RouterView />
  <SessionExpiredOverlay />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import SessionExpiredOverlay from '@/components/ui/SessionExpiredOverlay.vue'
</script>
