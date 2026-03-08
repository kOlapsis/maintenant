<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref } from 'vue'
import { usePostureStore } from '@/stores/posture'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'
import { timeAgo } from '@/utils/time'
import PostureScoreBadge from '@/components/PostureScoreBadge.vue'
import PostureContainerList from '@/components/PostureContainerList.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import ContainerDetail from '@/components/ContainerDetail.vue'
import FeatureGate from '@/components/FeatureGate.vue'
import { ShieldCheck, AlertTriangle } from 'lucide-vue-next'

const { hasFeature } = useEdition()
const store = usePostureStore()
const containerStore = useContainersStore()

const isAvailable = computed(() => hasFeature('security_posture'))
const posture = computed(() => store.posture)

const detailOpen = ref(false)
const selectedContainerId = ref<number | null>(null)

function openDetail(containerId: number) {
  selectedContainerId.value = containerId
  detailOpen.value = true
}

const selectedContainerName = computed(() => {
  if (!selectedContainerId.value) return ''
  const c = containerStore.allContainers.find(ct => ct.id === selectedContainerId.value)
  return c?.name ?? ''
})

onMounted(() => {
  if (isAvailable.value) {
    store.fetchPosture()
    store.connectSSE()
    containerStore.fetchContainers()
  }
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-bold text-white">Security Posture</h1>
          <p class="text-xs text-slate-500 mt-0.5">
            <template v-if="posture">{{ posture.scored_count }}/{{ posture.container_count }} containers scored</template>
            <template v-else>Infrastructure-wide security scoring</template>
          </p>
        </div>
        <div v-if="posture" class="flex items-center gap-3">
          <span v-if="posture.is_partial" class="flex items-center gap-1.5 text-[10px] text-amber-400 font-bold">
            <AlertTriangle :size="11" />
            Partial data
          </span>
          <span class="text-[10px] text-slate-500 font-bold">
            Updated {{ timeAgo(posture.computed_at) }}
          </span>
        </div>
      </div>

      <!-- Enterprise gate -->
      <FeatureGate
        feature="security_posture"
        title="Security Posture"
        description="View infrastructure-wide security scoring with per-container breakdown, category analysis, and risk acknowledgments."
      >

        <!-- Loading -->
        <template v-if="store.loading && !posture">
          <div class="space-y-4">
            <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
              <div v-for="i in 6" :key="i" class="h-24 animate-pulse rounded-xl bg-slate-800/50" />
            </div>
          </div>
        </template>

        <!-- Posture dashboard -->
        <template v-else-if="posture">
          <!-- Score + Category summary -->
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            <!-- Global score card -->
            <div class="bg-[#12151C] rounded-xl p-4 border border-slate-800 flex flex-col items-center justify-center">
              <PostureScoreBadge :score="posture.score" :color="posture.color" size="md" />
              <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-2">Score</p>
            </div>

            <!-- Category cards -->
            <div
              v-for="cat in posture.categories"
              :key="cat.name"
              class="bg-[#12151C] rounded-xl p-4 border border-slate-800"
            >
              <div class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">{{ cat.name.replace('_', ' ') }}</div>
              <p class="text-2xl font-black" :class="cat.total_issues > 0 ? 'text-amber-400' : 'text-slate-600'">
                {{ cat.total_issues }}
              </p>
              <p class="text-[10px] text-slate-600 mt-0.5">{{ cat.summary }}</p>
            </div>
          </div>

          <!-- Top risks -->
          <div v-if="posture.top_risks.length > 0">
            <h2 class="text-sm font-bold text-white mb-3">Top Risks</h2>
            <PostureContainerList :risks="posture.top_risks" @select="openDetail" />
          </div>
        </template>

        <!-- No data -->
        <div v-else class="flex flex-col items-center justify-center py-16">
          <ShieldCheck :size="40" class="text-slate-700 mb-3" />
          <p class="text-sm text-slate-600 font-medium">No posture data available</p>
          <p class="text-[10px] text-slate-700 mt-1">Make sure containers are being monitored</p>
        </div>

      </FeatureGate>

      <!-- Container detail slide-over -->
      <SlideOverPanel
        v-model:open="detailOpen"
        :title="selectedContainerName"
        width="max-w-2xl"
      >
        <template #header>
          <span></span>
        </template>
        <ContainerDetail
          v-if="selectedContainerId"
          :container-id="selectedContainerId"
          @close="detailOpen = false"
          @deleted="detailOpen = false"
        />
      </SlideOverPanel>
    </div>
  </div>
</template>
