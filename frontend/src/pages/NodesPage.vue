<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { useFleetRuntimes } from '@/composables/useFleetRuntimes'
import { useResourcesStore } from '@/stores/resources'
import FeatureGate from '@/components/FeatureGate.vue'
import K8sNodeList from '@/components/K8sNodeList.vue'
import SwarmNodeList from '@/components/SwarmNodeList.vue'

// Runtime views follow the selected host scope (an agent's runtime, or the union
// in "all"), not the server's own runtime.
const { availableRuntimes } = useFleetRuntimes()
const resources = useResourcesStore()
const isKubernetes = computed(() => availableRuntimes.value.includes('kubernetes'))
const isSwarm = computed(() => availableRuntimes.value.includes('swarm'))

function onK8sNodeSelect(name: string) {
  // K8s node detail view can be wired in a future phase.
  console.log('Selected K8s node:', name)
}

function onSwarmNodeSelect(nodeId: string) {
  // Swarm node detail view can be wired in a future phase.
  console.log('Selected Swarm node:', nodeId)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-black text-pb-primary">Nodes</h1>
        <p class="mt-1 text-sm text-pb-muted">Node health and capacity across your cluster</p>
      </div>
    </div>

    <!-- K8s nodes (Enterprise) -->
    <template v-if="isKubernetes">
      <FeatureGate
        feature="k8s_cluster"
        title="Kubernetes Node Management"
        description="View node status, roles, capacity, and conditions across your cluster."
      >
        <K8sNodeList :key="`k8s-${resources.selected}`" @select="onK8sNodeSelect" />
      </FeatureGate>
    </template>

    <!-- Swarm nodes (Enterprise) -->
    <template v-if="isSwarm">
      <FeatureGate
        feature="swarm_dashboard"
        title="Swarm Node Management"
        description="Monitor node availability, roles, and task distribution across your Swarm cluster."
      >
        <SwarmNodeList :key="`swarm-${resources.selected}`" @select="onSwarmNodeSelect" />
      </FeatureGate>
    </template>

    <!-- Neither runtime in the selected scope -->
    <template v-if="!isKubernetes && !isSwarm">
      <p class="text-sm text-slate-400">Node management is available for Kubernetes and Docker Swarm runtimes.</p>
    </template>
    </div>
  </div>
</template>
