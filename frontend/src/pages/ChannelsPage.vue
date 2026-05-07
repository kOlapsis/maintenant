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
import { onMounted, onUnmounted } from 'vue'
import ChannelManager from '@/components/ChannelManager.vue'
import { useChannelsStore } from '@/stores/channels'

const store = useChannelsStore()

onMounted(() => {
  store.fetchChannels()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">
      <!-- Header -->
      <div>
        <h1 class="text-xl font-bold text-pb-primary">Channels</h1>
        <p class="text-xs text-slate-500 mt-0.5">
          Pure delivery targets — silent until wired through Triggers or Escalations
        </p>
      </div>

      <ChannelManager />
    </div>
  </div>
</template>
