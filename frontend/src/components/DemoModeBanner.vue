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
import AlertBanner from '@/components/ui/AlertBanner.vue'
import { useEdition } from '@/composables/useEdition'

const { isDemo } = useEdition()
</script>

<template>
  <AlertBanner v-if="isDemo" severity="info" label="DEMO MODE" class="shrink-0">
    Demo mode: read-only. You can explore everything, changes are disabled.
  </AlertBanner>
</template>
