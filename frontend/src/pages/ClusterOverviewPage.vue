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
import K8sClusterOverview from '@/components/K8sClusterOverview.vue'
import SwarmClusterOverview from '@/components/SwarmClusterOverview.vue'
import IncidentFeedCard from '@/components/dashboard/IncidentFeedCard.vue'
import HostResourcesCard from '@/components/dashboard/HostResourcesCard.vue'

// Runtime views follow the selected host scope, not the server's own runtime.
const { availableRuntimes, kubernetesOnly } = useFleetRuntimes()
const resources = useResourcesStore()
const isKubernetes = computed(() => availableRuntimes.value.includes('kubernetes'))
const isSwarm = computed(() => availableRuntimes.value.includes('swarm'))
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Cluster Overview</h1>
        <p class="mt-1 text-sm text-mnt-muted">Aggregated cluster health, nodes, and workloads</p>
      </div>
    </div>

    <!-- K8s cluster overview (Pro) -->
    <template v-if="isKubernetes">
      <FeatureGate
        feature="k8s_cluster"
        title="Kubernetes Cluster Intelligence"
        description="Aggregated cluster health, node status, pod breakdown, and per-namespace summaries."
      >
        <K8sClusterOverview :key="`k8s-${resources.selected}`" />
      </FeatureGate>
    </template>

    <!-- Swarm cluster overview (Pro) -->
    <template v-if="isSwarm">
      <FeatureGate
        feature="swarm_dashboard"
        title="Swarm Cluster Intelligence"
        description="Real-time cluster health, node status, and service replica monitoring for Docker Swarm."
      >
        <SwarmClusterOverview :key="`swarm-${resources.selected}`" />
      </FeatureGate>
    </template>

    <!-- Neither runtime in the selected scope -->
    <template v-if="!isKubernetes && !isSwarm">
      <p class="text-sm text-mnt-muted">Cluster overview is available for Kubernetes and Docker Swarm runtimes.</p>
    </template>

    <!-- All-Kubernetes fleet: this page replaces the dashboard, so keep its
         alert feed and host (agent) resource gauges here. -->
    <div v-if="kubernetesOnly" class="grid grid-cols-1 lg:grid-cols-3 gap-3 sm:gap-5 mt-6">
      <IncidentFeedCard class="lg:col-span-2" />
      <HostResourcesCard :show-monitor-stats="false" />
    </div>
    </div>
  </div>
</template>
