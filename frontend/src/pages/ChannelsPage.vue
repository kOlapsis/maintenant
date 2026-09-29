<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
        <h1 class="text-2xl font-black text-mnt-primary">Channels</h1>
        <p class="mt-1 text-sm text-mnt-muted">
          Pure delivery targets — silent until wired through Triggers or Escalations
        </p>
      </div>

      <ChannelManager />
    </div>
  </div>
</template>
