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
import { useResourcesStore } from '@/stores/resources'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { type K8sPod } from '@/services/kubernetesApi'
import K8sPodList from '@/components/K8sPodList.vue'
import NamespaceSelector from '@/components/NamespaceSelector.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { Box } from 'lucide-vue-next'

const store = useKubernetesStore()
const namespacesStore = useNamespacesStore()
const resources = useResourcesStore()
const { openDetail } = inject(detailSlideOverKey)!

onMounted(async () => {
  store.startListening()
  await store.fetchPodsList()
})

onUnmounted(() => {
  store.stopListening()
})

// Refresh when namespace selection changes.
watch(
  () => namespacesStore.namespacesParam,
  () => {
    store.fetchPodsList()
  },
)

// Refresh namespaces + pods when the host scope changes.
watch(
  () => resources.selected,
  () => {
    namespacesStore.fetchNamespacesList()
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
          <h1 class="text-2xl font-black text-pb-primary">Pods</h1>
          <p class="mt-1 text-sm text-pb-muted">Kubernetes pods across all workloads</p>
        </div>
        <NamespaceSelector />
      </div>

      <!-- Loading -->
      <div v-if="store.loading && store.pods.length === 0" class="rounded-xl border border-pb-default bg-pb-surface p-4">
        <LoadingSkeleton variant="list" :count="6" />
      </div>

      <!-- Error -->
      <div v-else-if="store.error" class="overflow-hidden rounded-xl border border-pb-default bg-pb-surface">
        <ErrorState :message="store.error" />
      </div>

      <!-- Empty -->
      <div v-else-if="store.pods.length === 0" class="overflow-hidden rounded-xl border border-pb-default bg-pb-surface">
        <EmptyState
          :icon="Box"
          title="No pods found"
          description="Make sure the Kubernetes cluster is reachable and pods are running."
        />
      </div>

      <!-- Pod list -->
      <K8sPodList
        v-else
        :pods="store.pods"
        @select="handleSelect"
      />

      <!-- Footer count -->
      <div v-if="store.pods.length > 0" class="mt-4 text-xs text-pb-muted text-right tabular-nums">
        {{ store.pods.length }} pod{{ store.pods.length === 1 ? '' : 's' }}
      </div>
    </div>
  </div>
</template>
