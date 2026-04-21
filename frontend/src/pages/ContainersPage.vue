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
import { inject, computed } from 'vue'
import ContainerList from '@/components/ContainerList.vue'
import ResourceSummary from '@/components/ResourceSummary.vue'
import { useContainersStore } from '@/stores/containers'
import { useUpdatesStore } from '@/stores/updates'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import type { Container } from '@/services/containerApi'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'

const store = useContainersStore()
const updatesStore = useUpdatesStore()
const { openDetail } = inject(detailSlideOverKey)!

updatesStore.fetchAllUpdates()
const isK8s = computed(() => store.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')

function handleSelect(container: Container) {
  openDetail('container', container.id)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-pb-primary">Containers</h1>
      <p class="mt-1 text-sm text-slate-500">
        Auto-discovered {{ store.runtimeLabel }} containers
      </p>
    </div>

    <!-- Runtime unavailable warning -->
    <InlineAlert
      v-if="!store.runtimeConnected"
      severity="critical"
      tag="OFFLINE"
      class="mb-6"
    >
      <template #title>{{ store.runtimeLabel }} runtime unavailable</template>
      Cannot connect to the container runtime. Check that maintenant has access to the {{ store.runtimeLabel }} API.
    </InlineAlert>

    <FeatureHint
      v-if="store.runtimeConnected"
      storage-key="containers"
      legacy-storage-key="pb:hideLabelTips"
      :title="`Customize with ${labelOrAnnotation}s`"
      :doc-href="docUrl(isK8s ? 'features/containers/#grouping' : 'features/containers/#auto-discovery')"
    >
      Use {{ labelOrAnnotation }}s to configure container behavior:
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.ignore</code> to hide a container,
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.group</code> to group containers,
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.alert.severity</code> to set alert severity.
    </FeatureHint>

    <ResourceSummary />
    <ContainerList @select="handleSelect" />

  </div>
  </div>
</template>
