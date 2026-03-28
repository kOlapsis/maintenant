<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { useRuntime } from '@/composables/useRuntime'
import FeatureGate from '@/components/FeatureGate.vue'
import K8sClusterOverview from '@/components/K8sClusterOverview.vue'
import SwarmClusterOverview from '@/components/SwarmClusterOverview.vue'

const { isSwarm, isKubernetes } = useRuntime()
</script>

<template>
  <div class="p-6">
    <h1 class="text-xl font-bold text-white mb-6">Cluster Overview</h1>

    <!-- K8s cluster overview (Enterprise) -->
    <template v-if="isKubernetes">
      <FeatureGate
        feature="k8s_cluster"
        title="Kubernetes Cluster Intelligence"
        description="Aggregated cluster health, node status, pod breakdown, and per-namespace summaries."
      >
        <K8sClusterOverview />
      </FeatureGate>
    </template>

    <!-- Swarm cluster overview (Enterprise) -->
    <template v-else-if="isSwarm">
      <FeatureGate
        feature="swarm_dashboard"
        title="Swarm Cluster Intelligence"
        description="Real-time cluster health, node status, and service replica monitoring for Docker Swarm."
      >
        <SwarmClusterOverview />
      </FeatureGate>
    </template>

    <!-- Docker standalone -->
    <template v-else>
      <p class="text-sm text-slate-400">Cluster overview is available for Kubernetes and Docker Swarm runtimes.</p>
    </template>
  </div>
</template>
