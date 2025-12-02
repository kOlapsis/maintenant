<script setup lang="ts">
import { ref } from 'vue'
import ContainerList from '@/components/ContainerList.vue'
import ContainerDetail from '@/components/ContainerDetail.vue'
import ResourceSummary from '@/components/ResourceSummary.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import { useContainersStore } from '@/stores/containers'
import type { Container } from '@/services/containerApi'
import { AlertTriangle } from 'lucide-vue-next'

const store = useContainersStore()

const selectedContainer = ref<Container | null>(null)
const detailOpen = ref(false)

function openDetail(container: Container) {
  selectedContainer.value = container
  detailOpen.value = true
}
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-white">Containers</h1>
      <p class="mt-1 text-sm text-slate-500">
        Auto-discovered {{ store.runtimeLabel }} containers
      </p>
    </div>

    <!-- Runtime unavailable warning -->
    <div
      v-if="!store.runtimeConnected"
      class="mb-6 rounded-2xl p-4 bg-amber-500/10 border border-amber-500/30"
    >
      <div class="flex items-start gap-3">
        <AlertTriangle :size="20" class="text-amber-500 shrink-0 mt-0.5" />
        <div>
          <h3 class="text-sm font-medium text-amber-400">
            {{ store.runtimeLabel }} runtime unavailable
          </h3>
          <p class="mt-1 text-sm text-slate-400">
            Cannot connect to the container runtime. Check that PulseBoard has access to the {{ store.runtimeLabel }} API.
          </p>
        </div>
      </div>
    </div>

    <ResourceSummary />
    <ContainerList @select="openDetail" />

    <!-- Container detail slide-over -->
    <SlideOverPanel
      v-model:open="detailOpen"
      :title="selectedContainer?.name || ''"
      width="max-w-2xl"
    >
      <template #header>
        <span></span>
      </template>
      <ContainerDetail
        v-if="selectedContainer"
        :container-id="selectedContainer.id"
        @close="detailOpen = false"
      />
    </SlideOverPanel>
  </div>
</template>
