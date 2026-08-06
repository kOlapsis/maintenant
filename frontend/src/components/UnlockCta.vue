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
import { RouterLink } from 'vue-router'

/**
 * The call to action on a locked feature.
 *
 * It deliberately names no tier. Most capabilities are opened by more than one
 * edition — anything Personal covers, Pro covers too — so naming one of them
 * would be both awkward and incomplete. The button sends the reader to the
 * comparison, which is where the choice belongs; the pill next to the feature
 * already says which edition is the minimum.
 */
</script>

<template>
  <RouterLink
    :to="{ name: 'editions' }"
    class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-semibold bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted shadow-lg shadow-mnt-green-500/20 transition-colors"
  >
    Unlock with a license
  </RouterLink>
</template>
