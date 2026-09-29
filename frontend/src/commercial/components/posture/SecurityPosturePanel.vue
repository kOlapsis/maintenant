<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { inject, onMounted, onUnmounted, computed } from 'vue'
import { usePostureStore } from '@/commercial/stores/posture'
import { useContainersStore } from '@/stores/containers'
import PostureScoreBadge from './PostureScoreBadge.vue'
import PostureContainerList from './PostureContainerList.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'

import { ShieldCheck } from 'lucide-vue-next'

const store = usePostureStore()
const containerStore = useContainersStore()
const { openDetail } = inject(detailSlideOverKey)!

const posture = computed(() => store.posture)

function handleSelectContainer(containerId: string) {
  openDetail('container', containerId)
}

onMounted(() => {
  store.fetchPosture()
  store.connectSSE()
  containerStore.fetchContainers()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <!-- Loading -->
  <template v-if="store.loading && !posture">
    <div class="space-y-4">
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        <div v-for="i in 6" :key="i" class="h-24 animate-pulse rounded-xl bg-mnt-elevated/50" />
      </div>
    </div>
  </template>

  <!-- Posture dashboard -->
  <template v-else-if="posture">
    <!-- Score + Category summary -->
    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
      <!-- Global score card -->
      <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default flex flex-col items-center justify-center">
        <PostureScoreBadge :score="posture.score" :color="posture.color" size="md" />
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mt-2">Score</p>
      </div>

      <!-- Category cards -->
      <div
        v-for="cat in posture.categories"
        :key="cat.name"
        class="bg-mnt-surface rounded-xl p-4 border border-mnt-default"
      >
        <div class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">{{ cat.name.replace('_', ' ') }}</div>
        <p class="text-2xl font-black" :class="cat.total_issues > 0 ? 'text-mnt-status-warn' : 'text-mnt-muted'">
          {{ cat.total_issues }}
        </p>
        <p class="text-[10px] text-mnt-muted mt-0.5">{{ cat.summary }}</p>
      </div>
    </div>

    <!-- Top risks -->
    <div v-if="posture.top_risks.length > 0">
      <h2 class="text-sm font-bold text-mnt-primary mb-3">Top Risks</h2>
      <PostureContainerList :risks="posture.top_risks" @select="handleSelectContainer" />
    </div>
  </template>

  <!-- No data -->
  <div v-else class="flex flex-col items-center justify-center py-16">
    <ShieldCheck :size="40" class="text-mnt-muted mb-3" />
    <p class="text-sm text-mnt-muted font-medium">No posture data available</p>
    <p class="text-[10px] text-mnt-muted mt-1">Make sure containers are being monitored</p>
  </div>
</template>
