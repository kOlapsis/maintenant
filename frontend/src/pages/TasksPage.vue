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
import { ref, computed, onMounted } from 'vue'
import {
  fetchSwarmTasks,
  fetchSwarmServices,
  type SwarmTaskResponse,
  type SwarmServiceResponse,
} from '@/services/swarmApi'
import { timeAgo } from '@/utils/time'
import { ClipboardList } from 'lucide-vue-next'

type TaskWithService = SwarmTaskResponse & { service_name: string }

const tasks = ref<TaskWithService[]>([])
const services = ref<SwarmServiceResponse[]>([])
const loading = ref(true)

const filterService = ref('')
const filterState = ref('')

const TASK_STATES = ['running', 'complete', 'failed', 'rejected', 'shutdown', 'starting', 'preparing', 'assigned']

const filteredTasks = computed(() => {
  return tasks.value.filter(t => {
    if (filterService.value && t.service_name !== filterService.value) return false
    if (filterState.value && t.state !== filterState.value) return false
    return true
  })
})

onMounted(async () => {
  try {
    const [tasksResp, servicesResp] = await Promise.all([
      fetchSwarmTasks(),
      fetchSwarmServices(),
    ])
    tasks.value = tasksResp.tasks
    services.value = servicesResp.services
  } finally {
    loading.value = false
  }
})

function stateColor(state: string): string {
  switch (state) {
    case 'running': return 'text-emerald-400'
    case 'complete': return 'text-slate-400'
    case 'failed': return 'text-red-400'
    case 'rejected': return 'text-red-400'
    case 'shutdown': return 'text-slate-500'
    case 'preparing':
    case 'starting':
    case 'assigned': return 'text-sky-400'
    default: return 'text-amber-400'
  }
}

function stateDot(state: string): string {
  switch (state) {
    case 'running': return 'bg-emerald-500'
    case 'failed':
    case 'rejected': return 'bg-red-500'
    case 'complete': return 'bg-slate-500'
    case 'preparing':
    case 'starting':
    case 'assigned': return 'bg-sky-500'
    default: return 'bg-amber-500'
  }
}

function shortId(id: string): string {
  return id.slice(0, 12)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
      <!-- Page header -->
      <div class="mb-6">
        <h1 class="text-2xl font-black text-pb-primary">Tasks</h1>
        <p class="mt-1 text-sm text-slate-500">All Swarm tasks</p>
      </div>

      <!-- Filter bar -->
      <div class="flex flex-wrap items-center gap-3 mb-4">
        <div class="flex items-center gap-2">
          <label class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Service</label>
          <select
            v-model="filterService"
            class="bg-pb-surface border border-slate-800 text-xs text-pb-secondary rounded-lg px-3 py-1.5 focus:outline-none focus:border-slate-600 cursor-pointer"
          >
            <option value="">All services</option>
            <option v-for="svc in services" :key="svc.service_id" :value="svc.name">
              {{ svc.name }}
            </option>
          </select>
        </div>

        <div class="flex items-center gap-2">
          <label class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">State</label>
          <select
            v-model="filterState"
            class="bg-pb-surface border border-slate-800 text-xs text-pb-secondary rounded-lg px-3 py-1.5 focus:outline-none focus:border-slate-600 cursor-pointer"
          >
            <option value="">All states</option>
            <option v-for="state in TASK_STATES" :key="state" :value="state">
              {{ state }}
            </option>
          </select>
        </div>

        <span class="ml-auto text-xs text-slate-500">
          {{ filteredTasks.length }} task{{ filteredTasks.length === 1 ? '' : 's' }}
        </span>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <span class="text-sm text-slate-500">Loading tasks…</span>
      </div>

      <!-- Empty -->
      <div
        v-else-if="filteredTasks.length === 0"
        class="bg-pb-surface rounded-xl border border-slate-800 px-6 py-12 text-center"
      >
        <ClipboardList :size="32" class="mx-auto mb-3 text-slate-600" />
        <p class="text-sm text-slate-500">
          {{ tasks.length === 0 ? 'No tasks found' : 'No tasks match the selected filters' }}
        </p>
      </div>

      <!-- Task list -->
      <div v-else class="bg-pb-surface rounded-xl border border-slate-800 overflow-hidden">
        <!-- Table header -->
        <div class="grid grid-cols-[1fr_1fr_80px_1fr_120px_80px] gap-3 px-4 py-2 border-b border-slate-800">
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Task ID</span>
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Service</span>
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Slot</span>
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Node</span>
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">State</span>
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest text-right">When</span>
        </div>

        <!-- Rows -->
        <div class="divide-y divide-slate-800/60">
          <div
            v-for="task in filteredTasks"
            :key="task.task_id"
            class="grid grid-cols-[1fr_1fr_80px_1fr_120px_80px] gap-3 px-4 py-2.5 hover:bg-slate-800/25 transition-all items-center"
          >
            <!-- Task ID -->
            <span class="text-xs text-slate-400 font-mono truncate">{{ shortId(task.task_id) }}</span>

            <!-- Service name -->
            <span class="text-xs text-pb-secondary font-medium truncate">{{ task.service_name }}</span>

            <!-- Slot -->
            <span class="text-xs text-slate-400 font-mono">#{{ task.slot }}</span>

            <!-- Node -->
            <span class="text-xs text-slate-500 truncate">{{ task.node_hostname || '—' }}</span>

            <!-- State + error -->
            <div class="flex items-center gap-2 min-w-0">
              <div :class="['w-1.5 h-1.5 rounded-full flex-shrink-0', stateDot(task.state)]" />
              <span :class="['text-xs font-medium', stateColor(task.state)]">{{ task.state }}</span>
              <span
                v-if="task.error"
                class="text-[10px] text-red-400 truncate"
                :title="task.error"
              >
                {{ task.error }}
              </span>
              <span
                v-else-if="task.exit_code !== null && task.exit_code !== 0"
                class="text-[10px] text-red-400 flex-shrink-0"
              >
                exit {{ task.exit_code }}
              </span>
            </div>

            <!-- When -->
            <span class="text-xs text-slate-500 tabular-nums text-right">{{ timeAgo(task.timestamp) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
