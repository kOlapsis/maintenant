<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useKubernetesStore } from '@/stores/kubernetes'
import ClusterHealthBadge from '@/components/ClusterHealthBadge.vue'

const k8sStore = useKubernetesStore()

onMounted(() => {
  k8sStore.fetchCluster()
  k8sStore.startListening()
})

onUnmounted(() => {
  k8sStore.stopListening()
})

const overview = computed(() => k8sStore.clusterOverview)

const totalPods = computed(() => {
  if (!overview.value) return 0
  const s = overview.value.pod_status
  return s.running + s.pending + s.failed + s.succeeded + s.unknown
})

function podBarWidth(count: number): string {
  if (totalPods.value === 0) return '0%'
  return `${Math.max((count / totalPods.value) * 100, count > 0 ? 2 : 0)}%`
}
</script>

<template>
  <div>
    <!-- Loading -->
    <div v-if="k8sStore.clusterLoading && !overview" class="text-sm text-slate-500 py-12 text-center">
      Loading cluster overview...
    </div>

    <!-- Error -->
    <div v-else-if="k8sStore.error && !overview" class="text-sm text-red-400 py-12 text-center">
      {{ k8sStore.error }}
    </div>

    <!-- Content -->
    <div v-else-if="overview" class="space-y-6">
      <!-- Health + summary cards -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Cluster Health -->
        <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Cluster Health</p>
          <ClusterHealthBadge :health="overview.cluster_health" />
        </div>

        <!-- Namespaces -->
        <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Namespaces</p>
          <p class="text-2xl font-bold text-white tabular-nums">{{ overview.namespace_count }}</p>
        </div>

        <!-- Nodes -->
        <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Nodes</p>
          <div class="flex items-baseline gap-1.5">
            <span class="text-2xl font-bold text-white tabular-nums">{{ overview.node_ready_count }}</span>
            <span class="text-sm text-slate-400">/ {{ overview.node_count }} ready</span>
          </div>
        </div>

        <!-- Workloads -->
        <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Workloads</p>
          <div class="flex items-baseline gap-1.5">
            <span class="text-2xl font-bold text-white tabular-nums">{{ overview.workload_healthy }}</span>
            <span class="text-sm text-slate-400">/ {{ overview.workload_count }} healthy</span>
          </div>
        </div>
      </div>

      <!-- Pod status breakdown -->
      <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-3">Pod Status</p>
        <div class="flex items-center gap-4 mb-3">
          <span class="text-2xl font-bold text-white tabular-nums">{{ totalPods }}</span>
          <span class="text-sm text-slate-400">total pods</span>
        </div>

        <!-- Stacked bar -->
        <div class="flex h-2.5 rounded-full overflow-hidden bg-[#0B0E13] border border-slate-800">
          <div
            class="bg-emerald-500 transition-all"
            :style="{ width: podBarWidth(overview.pod_status.running) }"
          />
          <div
            class="bg-amber-500 transition-all"
            :style="{ width: podBarWidth(overview.pod_status.pending) }"
          />
          <div
            class="bg-red-500 transition-all"
            :style="{ width: podBarWidth(overview.pod_status.failed) }"
          />
          <div
            class="bg-sky-500 transition-all"
            :style="{ width: podBarWidth(overview.pod_status.succeeded) }"
          />
          <div
            class="bg-slate-500 transition-all"
            :style="{ width: podBarWidth(overview.pod_status.unknown) }"
          />
        </div>

        <!-- Legend -->
        <div class="flex flex-wrap gap-x-5 gap-y-1 mt-2.5 text-xs">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-emerald-500" />
            <span class="text-slate-400">Running</span>
            <span class="text-white font-medium tabular-nums">{{ overview.pod_status.running }}</span>
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-amber-500" />
            <span class="text-slate-400">Pending</span>
            <span class="text-white font-medium tabular-nums">{{ overview.pod_status.pending }}</span>
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-red-500" />
            <span class="text-slate-400">Failed</span>
            <span class="text-white font-medium tabular-nums">{{ overview.pod_status.failed }}</span>
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-sky-500" />
            <span class="text-slate-400">Succeeded</span>
            <span class="text-white font-medium tabular-nums">{{ overview.pod_status.succeeded }}</span>
          </span>
          <span v-if="overview.pod_status.unknown > 0" class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-slate-500" />
            <span class="text-slate-400">Unknown</span>
            <span class="text-white font-medium tabular-nums">{{ overview.pod_status.unknown }}</span>
          </span>
        </div>
      </div>

      <!-- Per-namespace summaries -->
      <div class="bg-[#12151C] rounded-xl border border-slate-800 px-5 py-4">
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-3">Namespaces</p>

        <div v-if="overview.namespaces.length === 0" class="text-sm text-slate-500 py-4 text-center">
          No namespaces found
        </div>

        <div v-else class="space-y-1">
          <div
            v-for="ns in overview.namespaces"
            :key="ns.name"
            class="flex items-center justify-between px-3 py-2 rounded-lg hover:bg-slate-800/25 transition-all"
          >
            <div class="flex items-center gap-2.5 min-w-0">
              <span
                :class="[
                  'w-2 h-2 rounded-full flex-shrink-0',
                  ns.healthy ? 'bg-emerald-500' : 'bg-amber-500',
                ]"
              />
              <span class="text-sm text-white font-medium truncate">{{ ns.name }}</span>
            </div>
            <div class="flex items-center gap-4 text-xs text-slate-400 flex-shrink-0 ml-4">
              <span class="tabular-nums">{{ ns.workload_count }} workloads</span>
              <span class="tabular-nums">{{ ns.pod_count }} pods</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
