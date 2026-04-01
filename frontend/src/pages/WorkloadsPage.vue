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
import { type K8sWorkload } from '@/services/kubernetesApi'
import K8sWorkloadList from '@/components/K8sWorkloadList.vue'
import NamespaceSelector from '@/components/NamespaceSelector.vue'
import { LayoutGrid } from 'lucide-vue-next'

const store = useKubernetesStore()
const namespacesStore = useNamespacesStore()
const { openDetail } = inject(detailSlideOverKey)!

onMounted(async () => {
  store.startListening()
  await store.fetchWorkloadsList()
})

onUnmounted(() => {
  store.stopListening()
})

// Refresh when namespace selection changes
watch(
  () => namespacesStore.namespacesParam,
  () => {
    store.fetchWorkloadsList()
  },
)

function handleSelect(workload: K8sWorkload) {
  openDetail('k8s-workload', workload.id)
}

const totalWorkloads = () =>
  store.workloadGroups.reduce((sum, g) => sum + g.workloads.length, 0)
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
      <!-- Page header -->
      <div class="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-black text-pb-primary">Workloads</h1>
          <p class="mt-1 text-sm text-slate-500">Kubernetes workloads grouped by namespace</p>
        </div>
        <NamespaceSelector />
      </div>

      <!-- Loading -->
      <div v-if="store.loading && store.workloadGroups.length === 0" class="flex items-center justify-center py-16">
        <span class="text-sm text-slate-500">Loading workloads…</span>
      </div>

      <!-- Error -->
      <div
        v-else-if="store.error"
        class="bg-pb-surface rounded-xl border border-red-900/40 px-6 py-4 text-sm text-red-400"
      >
        {{ store.error }}
      </div>

      <!-- Empty -->
      <div
        v-else-if="store.workloadGroups.length === 0"
        class="bg-pb-surface rounded-xl border border-slate-800 px-6 py-12 text-center"
      >
        <LayoutGrid :size="32" class="mx-auto mb-3 text-slate-600" />
        <p class="text-sm text-slate-500">No workloads found</p>
        <p class="mt-1 text-xs text-slate-600">Make sure the Kubernetes cluster is reachable and workloads are deployed</p>
      </div>

      <!-- Workload groups -->
      <K8sWorkloadList
        v-else
        :groups="store.workloadGroups"
        @select="handleSelect"
      />

      <!-- Footer count -->
      <div v-if="store.workloadGroups.length > 0" class="mt-4 text-xs text-slate-600 text-right tabular-nums">
        {{ totalWorkloads() }} workload{{ totalWorkloads() === 1 ? '' : 's' }} across {{ store.workloadGroups.length }} namespace{{ store.workloadGroups.length === 1 ? '' : 's' }}
      </div>
    </div>
  </div>
</template>
