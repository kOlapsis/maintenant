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
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ContainerList from '@/components/ContainerList.vue'
import ContainerDetail from '@/components/ContainerDetail.vue'
import ResourceSummary from '@/components/ResourceSummary.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import { useContainersStore } from '@/stores/containers'
import { useUpdatesStore } from '@/stores/updates'
import type { Container } from '@/services/containerApi'
import { AlertTriangle, Info } from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const store = useContainersStore()
const updatesStore = useUpdatesStore()

updatesStore.fetchAllUpdates()
const isK8s = computed(() => store.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')
const showLabelTips = ref(localStorage.getItem('pb:hideLabelTips') !== '1')

function dismissLabelTips() {
  showLabelTips.value = false
  localStorage.setItem('pb:hideLabelTips', '1')
}

const selectedContainer = ref<Container | null>(null)
const detailOpen = ref(false)

function openDetail(container: Container) {
  selectedContainer.value = container
  detailOpen.value = true
}

function openContainerById(id: number) {
  const container = store.allContainers.find((c) => c.id === id)
  if (container) {
    openDetail(container)
    router.replace({ query: {} })
  }
}

// Auto-open container from query param (e.g. ?selected=42)
watch(
  () => [route.query.selected, store.allContainers.length] as const,
  ([selected]) => {
    if (selected && store.allContainers.length > 0) {
      openContainerById(Number(selected))
    }
  },
  { immediate: true },
)
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
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
            Cannot connect to the container runtime. Check that maintenant has access to the {{ store.runtimeLabel }} API.
          </p>
        </div>
      </div>
    </div>

    <!-- Label tips info banner -->
    <div
      v-if="showLabelTips && store.runtimeConnected"
      class="mb-6 rounded-2xl p-4 bg-pb-green-500/10 border border-pb-green-500/20"
    >
      <div class="flex items-start gap-3">
        <Info :size="20" class="text-pb-green-400 shrink-0 mt-0.5" />
        <div class="flex-1">
          <h3 class="text-sm font-medium text-pb-green-400">Customize with {{ labelOrAnnotation }}s</h3>
          <p class="mt-1 text-sm text-slate-400">
            Use {{ labelOrAnnotation }}s to configure container behavior:
            <code class="rounded-md px-1.5 py-0.5 text-xs bg-slate-900 text-slate-300">maintenant.ignore</code> to hide a container,
            <code class="rounded-md px-1.5 py-0.5 text-xs bg-slate-900 text-slate-300">maintenant.group</code> to group containers,
            <code class="rounded-md px-1.5 py-0.5 text-xs bg-slate-900 text-slate-300">maintenant.alert.severity</code> to set alert severity.
          </p>
        </div>
        <button
          @click="dismissLabelTips()"
          class="text-slate-500 hover:text-slate-300 shrink-0"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="4" y1="4" x2="12" y2="12" /><line x1="12" y1="4" x2="4" y2="12" /></svg>
        </button>
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
        @deleted="detailOpen = false; store.fetchContainers()"
      />
    </SlideOverPanel>
  </div>
  </div>
</template>
