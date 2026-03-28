<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { useRuntime } from '@/composables/useRuntime'
import FeatureGate from '@/components/FeatureGate.vue'
import K8sNodeList from '@/components/K8sNodeList.vue'
import SwarmNodeList from '@/components/SwarmNodeList.vue'

const { isSwarm, isKubernetes } = useRuntime()

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
  <div class="p-6">
    <h1 class="text-xl font-bold text-white mb-6">Nodes</h1>

    <!-- K8s nodes (Enterprise) -->
    <template v-if="isKubernetes">
      <FeatureGate
        feature="k8s_cluster"
        title="Kubernetes Node Management"
        description="View node status, roles, capacity, and conditions across your cluster."
      >
        <K8sNodeList @select="onK8sNodeSelect" />
      </FeatureGate>
    </template>

    <!-- Swarm nodes (Enterprise) -->
    <template v-else-if="isSwarm">
      <FeatureGate
        feature="swarm_dashboard"
        title="Swarm Node Management"
        description="Monitor node availability, roles, and task distribution across your Swarm cluster."
      >
        <SwarmNodeList @select="onSwarmNodeSelect" />
      </FeatureGate>
    </template>

    <!-- Docker standalone -->
    <template v-else>
      <p class="text-sm text-slate-400">Node management is available for Kubernetes and Docker Swarm runtimes.</p>
    </template>
  </div>
</template>
