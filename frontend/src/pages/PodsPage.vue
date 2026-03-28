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
import { onMounted, onUnmounted, inject, watch } from 'vue'
import { useKubernetesStore } from '@/stores/kubernetes'
import { useNamespacesStore } from '@/stores/namespaces'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { type K8sPod } from '@/services/kubernetesApi'
import K8sPodList from '@/components/K8sPodList.vue'
import NamespaceSelector from '@/components/NamespaceSelector.vue'
import { Box } from 'lucide-vue-next'

const store = useKubernetesStore()
const namespacesStore = useNamespacesStore()
const { openDetail } = inject(detailSlideOverKey)!

onMounted(async () => {
  store.startListening()
  await store.fetchPodsList()
})

onUnmounted(() => {
  store.stopListening()
})

// Refresh when namespace selection changes
watch(
  () => namespacesStore.namespacesParam,
  () => {
    store.fetchPodsList()
  },
)

function handleSelect(pod: K8sPod) {
  openDetail('k8s-pod', `${pod.namespace}/${pod.name}`)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
      <!-- Page header -->
      <div class="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-white">Pods</h1>
          <p class="mt-1 text-sm text-slate-500">Kubernetes pods across all workloads</p>
        </div>
        <NamespaceSelector />
      </div>

      <!-- Loading -->
      <div v-if="store.loading && store.pods.length === 0" class="flex items-center justify-center py-16">
        <span class="text-sm text-slate-500">Loading pods…</span>
      </div>

      <!-- Error -->
      <div
        v-else-if="store.error"
        class="bg-[#12151C] rounded-xl border border-red-900/40 px-6 py-4 text-sm text-red-400"
      >
        {{ store.error }}
      </div>

      <!-- Empty -->
      <div
        v-else-if="store.pods.length === 0"
        class="bg-[#12151C] rounded-xl border border-slate-800 px-6 py-12 text-center"
      >
        <Box :size="32" class="mx-auto mb-3 text-slate-600" />
        <p class="text-sm text-slate-500">No pods found</p>
        <p class="mt-1 text-xs text-slate-600">Make sure the Kubernetes cluster is reachable and pods are running</p>
      </div>

      <!-- Pod list -->
      <K8sPodList
        v-else
        :pods="store.pods"
        @select="handleSelect"
      />

      <!-- Footer count -->
      <div v-if="store.pods.length > 0" class="mt-4 text-xs text-slate-600 text-right tabular-nums">
        {{ store.pods.length }} pod{{ store.pods.length === 1 ? '' : 's' }}
      </div>
    </div>
  </div>
</template>
