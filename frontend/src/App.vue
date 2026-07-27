<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<template>
  <RouterView />
  <SessionExpiredOverlay />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import SessionExpiredOverlay from '@/components/ui/SessionExpiredOverlay.vue'
</script>
