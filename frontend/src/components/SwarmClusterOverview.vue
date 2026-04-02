<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { onMounted, onUnmounted, computed } from 'vue'
import { useSwarmStore } from '@/stores/swarm'
import ClusterHealthBadge from '@/components/ClusterHealthBadge.vue'
import { Server, Layers, Activity, AlertTriangle } from 'lucide-vue-next'

const store = useSwarmStore()

onMounted(() => {
  store.loadCluster()
  store.startListening()
})

onUnmounted(() => {
  store.stopListening()
})

const alertCount = computed(() => {
  return store.crashLoops.size + store.updateProgress.size
})
</script>

<template>
  <div>
    <div v-if="store.clusterLoading && !store.cluster" class="text-sm text-slate-500 py-8 text-center">
      Loading cluster data...
    </div>

    <div v-else-if="store.clusterError" class="text-sm text-red-400 py-8 text-center">
      {{ store.clusterError }}
    </div>

    <div v-else-if="store.cluster" class="space-y-4">
      <!-- Health + Cluster ID -->
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3">
          <ClusterHealthBadge :health="store.cluster.cluster_health" />
          <span class="text-xs text-slate-500 font-mono truncate max-w-48" :title="store.cluster.cluster_id">
            {{ store.cluster.cluster_id }}
          </span>
        </div>
        <div v-if="alertCount > 0" class="flex items-center gap-1.5 text-amber-400">
          <AlertTriangle class="h-3.5 w-3.5" />
          <span class="text-xs font-medium">{{ alertCount }} alert{{ alertCount > 1 ? 's' : '' }}</span>
        </div>
      </div>

      <!-- Summary cards -->
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <!-- Nodes -->
        <div class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <Server class="h-3.5 w-3.5 text-slate-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Nodes</span>
          </div>
          <div class="text-xl font-bold text-pb-primary tabular-nums">
            {{ store.cluster.manager_count + store.cluster.worker_count }}
          </div>
          <div class="text-xs text-slate-400 mt-1">
            {{ store.cluster.manager_count }} mgr / {{ store.cluster.worker_count }} wkr
          </div>
        </div>

        <!-- Services -->
        <div class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <Layers class="h-3.5 w-3.5 text-slate-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Services</span>
          </div>
          <div class="text-xl font-bold text-pb-primary tabular-nums">
            {{ store.cluster.total_services }}
          </div>
        </div>

        <!-- Tasks -->
        <div class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <Activity class="h-3.5 w-3.5 text-slate-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Tasks</span>
          </div>
          <div class="flex items-baseline gap-1">
            <span class="text-xl font-bold tabular-nums" :class="store.cluster.running_tasks === store.cluster.desired_tasks ? 'text-pb-primary' : 'text-amber-400'">
              {{ store.cluster.running_tasks }}
            </span>
            <span class="text-xs text-slate-500">/ {{ store.cluster.desired_tasks }}</span>
          </div>
        </div>

        <!-- Node Status -->
        <div class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Node Status</span>
          </div>
          <div class="flex items-center gap-3 text-xs">
            <span class="text-pb-status-ok tabular-nums">{{ store.cluster.nodes.ready }} ready</span>
            <span v-if="store.cluster.nodes.down > 0" class="text-red-400 tabular-nums">
              {{ store.cluster.nodes.down }} down
            </span>
            <span v-if="store.cluster.nodes.disconnected > 0" class="text-amber-400 tabular-nums">
              {{ store.cluster.nodes.disconnected }} disc.
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
