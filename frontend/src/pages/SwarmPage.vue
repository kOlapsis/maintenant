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
import { ref, onMounted, onUnmounted } from 'vue'
import FeatureGate from '@/components/FeatureGate.vue'
import SwarmNodeList from '@/components/SwarmNodeList.vue'
import SwarmServiceList from '@/components/SwarmServiceList.vue'
import { fetchSwarmDashboard, type SwarmDashboardResponse } from '@/services/swarmApi'
import { useSwarmStore } from '@/stores/swarm'

const swarmStore = useSwarmStore()
const dashboard = ref<SwarmDashboardResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

async function loadDashboard() {
  loading.value = true
  error.value = null
  try {
    dashboard.value = await fetchSwarmDashboard()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load dashboard'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  swarmStore.loadInfo()
  swarmStore.startListening()
  loadDashboard()
})

onUnmounted(() => {
  swarmStore.stopListening()
})
</script>

<template>
  <FeatureGate
    feature="swarm_dashboard"
    title="Swarm Cluster Dashboard"
    description="Get a complete view of your Docker Swarm cluster with node health monitoring, service management, crash-loop detection, and rolling update tracking."
  >
    <div class="space-y-6">
      <!-- Cluster Summary -->
      <div v-if="dashboard" class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold text-white tabular-nums">{{ dashboard.cluster.manager_count }}</p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Managers</p>
        </div>
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold text-white tabular-nums">{{ dashboard.cluster.worker_count }}</p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Workers</p>
        </div>
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold text-white tabular-nums">{{ dashboard.cluster.service_count }}</p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Services</p>
        </div>
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold text-white tabular-nums">{{ dashboard.cluster.task_count }}</p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Tasks</p>
        </div>
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold tabular-nums" :class="dashboard.cluster.healthy_task_count === dashboard.cluster.task_count ? 'text-emerald-400' : 'text-amber-400'">
            {{ dashboard.cluster.healthy_task_count }}
          </p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Healthy</p>
        </div>
        <div class="bg-[#12151C] rounded-xl border border-slate-800 p-4 text-center">
          <p class="text-2xl font-bold tabular-nums" :class="(dashboard.cluster.task_count - dashboard.cluster.healthy_task_count) === 0 ? 'text-slate-500' : 'text-red-400'">
            {{ dashboard.cluster.task_count - dashboard.cluster.healthy_task_count }}
          </p>
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mt-1">Unhealthy</p>
        </div>
      </div>

      <div v-if="loading && !dashboard" class="text-sm text-slate-500 py-8 text-center">
        Loading dashboard...
      </div>

      <div v-if="error" class="text-sm text-red-400 py-4 text-center">
        {{ error }}
      </div>

      <!-- Two-column layout: Nodes + Services -->
      <div v-if="dashboard" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div>
          <SwarmNodeList />
        </div>
        <div>
          <SwarmServiceList :services="dashboard.services" />
        </div>
      </div>
    </div>
  </FeatureGate>
</template>
