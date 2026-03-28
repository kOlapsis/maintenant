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
import { ref, onMounted } from 'vue'
import {
  fetchSwarmServiceDetail,
  fetchSwarmServiceResources,
  type SwarmServiceDetailResponse,
  type SwarmTaskResourceEntry,
} from '@/services/swarmApi'
import SwarmTaskList from './SwarmTaskList.vue'
import FeatureGate from './FeatureGate.vue'
import { timeAgo } from '@/utils/time'
import { useEdition } from '@/composables/useEdition'

const props = defineProps<{
  serviceId: string
}>()

defineEmits<{
  close: []
}>()

type Tab = 'tasks' | 'ports' | 'networks' | 'resources'

const { hasFeature } = useEdition()
const detail = ref<SwarmServiceDetailResponse | null>(null)
const loading = ref(true)
const activeTab = ref<Tab>('tasks')
const taskResources = ref<SwarmTaskResourceEntry[]>([])
const resourcesLoading = ref(false)

onMounted(async () => {
  try {
    detail.value = await fetchSwarmServiceDetail(props.serviceId)
  } finally {
    loading.value = false
  }
})

async function loadResources() {
  if (taskResources.value.length > 0) return
  resourcesLoading.value = true
  try {
    const resp = await fetchSwarmServiceResources(props.serviceId)
    taskResources.value = resp.tasks
  } catch {
    // silently handle — empty state will show
  } finally {
    resourcesLoading.value = false
  }
}

function onTabClick(tab: Tab) {
  activeTab.value = tab
  if (tab === 'resources') loadResources()
}

function replicaColor(running: number, desired: number): string {
  if (running >= desired) return 'text-emerald-400'
  if (running > 0) return 'text-amber-400'
  return 'text-red-400'
}

function imageTag(image: string): { name: string; tag: string } {
  const colonIdx = image.lastIndexOf(':')
  if (colonIdx < 0) return { name: image, tag: 'latest' }
  return { name: image.slice(0, colonIdx), tag: image.slice(colonIdx + 1) }
}

function formatBytes(bytes: number | null): string {
  if (bytes === null || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function cpuBarWidth(cpu: number | null): string {
  if (cpu === null) return '0%'
  return `${Math.min(cpu, 100)}%`
}

function cpuColor(cpu: number | null): string {
  if (cpu === null) return 'bg-slate-700'
  if (cpu > 80) return 'bg-red-500'
  if (cpu > 50) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function memColor(percent: number | null): string {
  if (percent === null) return 'bg-slate-700'
  if (percent > 85) return 'bg-red-500'
  if (percent > 60) return 'bg-amber-500'
  return 'bg-sky-500'
}

const baseTabs: { key: Tab; label: string }[] = [
  { key: 'tasks', label: 'Tasks' },
  { key: 'ports', label: 'Ports' },
  { key: 'networks', label: 'Networks' },
]

const tabs = hasFeature('swarm_dashboard')
  ? [...baseTabs, { key: 'resources' as Tab, label: 'Resources' }]
  : baseTabs
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-16">
      <span class="text-sm text-slate-500">Loading service…</span>
    </div>

    <template v-else-if="detail">
      <!-- Header -->
      <div class="px-6 pt-4 pb-3 border-b border-slate-800">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h2 class="text-base font-bold text-white truncate">{{ detail.name }}</h2>
            <div class="flex items-center gap-2 mt-1 flex-wrap">
              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
                {{ detail.mode }}
              </span>
              <span
                v-if="detail.stack_name"
                class="text-[10px] font-bold uppercase tracking-wider text-sky-400 bg-sky-400/10 border border-sky-400/20 px-1.5 py-0.5 rounded"
              >
                {{ detail.stack_name }}
              </span>
              <span :class="['text-xs font-semibold tabular-nums', replicaColor(detail.running_replicas, detail.desired_replicas)]">
                {{ detail.running_replicas }}/{{ detail.desired_replicas }} replicas
              </span>
            </div>
          </div>
        </div>

        <!-- Image -->
        <div class="mt-3">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">Image</p>
          <div class="flex items-center gap-2">
            <span class="text-xs text-slate-300 font-mono truncate">{{ imageTag(detail.image).name }}</span>
            <span class="text-[10px] font-bold text-slate-500 bg-slate-800 px-1.5 py-0.5 rounded font-mono">
              {{ imageTag(detail.image).tag }}
            </span>
          </div>
        </div>

        <!-- Timestamps -->
        <div class="mt-3 flex gap-6 text-xs text-slate-500">
          <span>Created <span class="text-slate-400">{{ timeAgo(detail.created_at) }}</span></span>
          <span v-if="detail.update_status?.state">
            Update <span :class="detail.update_status.state === 'completed' ? 'text-emerald-400' : 'text-amber-400'">{{ detail.update_status.state }}</span>
          </span>
        </div>
      </div>

      <!-- Tabs -->
      <div class="flex border-b border-slate-800 px-6">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          :class="[
            'px-4 py-2.5 text-xs font-bold uppercase tracking-widest border-b-2 -mb-px transition-colors',
            activeTab === tab.key
              ? 'border-pb-green-400 text-pb-green-400'
              : 'border-transparent text-slate-500 hover:text-slate-300',
          ]"
          @click="onTabClick(tab.key)"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- Tab content -->
      <div class="flex-1 overflow-y-auto px-6 py-4">
        <!-- Tasks tab -->
        <template v-if="activeTab === 'tasks'">
          <SwarmTaskList :tasks="detail.tasks" />
        </template>

        <!-- Ports tab -->
        <template v-else-if="activeTab === 'ports'">
          <div v-if="detail.ports.length === 0" class="text-sm text-slate-500 py-4 text-center">
            No published ports
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="(port, i) in detail.ports"
              :key="i"
              class="bg-[#0B0E13] rounded-lg border border-slate-800 px-4 py-3 flex items-center justify-between"
            >
              <div class="flex items-center gap-3">
                <span class="text-sm font-mono text-white">
                  {{ port.published_port ? `:${port.published_port}` : 'auto' }}
                  <span class="text-slate-500 mx-1">→</span>
                  :{{ port.target_port }}
                </span>
                <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
                  {{ port.protocol }}
                </span>
              </div>
              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500">
                {{ port.publish_mode }}
              </span>
            </div>
          </div>
        </template>

        <!-- Networks tab -->
        <template v-else-if="activeTab === 'networks'">
          <div v-if="detail.networks.length === 0" class="text-sm text-slate-500 py-4 text-center">
            No networks attached
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="net in detail.networks"
              :key="net.network_id"
              class="bg-[#0B0E13] rounded-lg border border-slate-800 px-4 py-3 flex items-center justify-between"
            >
              <span class="text-sm text-white font-medium">{{ net.network_name }}</span>
              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
                {{ net.scope }}
              </span>
            </div>
          </div>
        </template>

        <!-- Resources tab (Enterprise) -->
        <template v-else-if="activeTab === 'resources'">
          <FeatureGate
            feature="swarm_dashboard"
            title="Task Resource Metrics"
            description="View per-task CPU, memory, and network usage for Swarm services."
          >
            <div v-if="resourcesLoading" class="text-sm text-slate-500 py-4 text-center">
              Loading resources...
            </div>
            <div v-else-if="taskResources.length === 0" class="text-sm text-slate-500 py-4 text-center">
              No resource data available
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="tr in taskResources"
                :key="tr.task_id"
                class="bg-[#0B0E13] rounded-lg border border-slate-800 px-4 py-3"
              >
                <div class="flex items-center justify-between mb-2">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-semibold text-white">Slot {{ tr.slot }}</span>
                    <span v-if="tr.node_hostname" class="text-xs text-slate-500 font-mono">{{ tr.node_hostname }}</span>
                  </div>
                  <span v-if="tr.timestamp" class="text-[10px] text-slate-600">{{ timeAgo(tr.timestamp) }}</span>
                </div>

                <!-- CPU bar -->
                <div class="mb-2">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">CPU</span>
                    <span class="text-xs text-slate-400 tabular-nums">
                      {{ tr.cpu_percent !== null ? `${tr.cpu_percent.toFixed(1)}%` : '-' }}
                    </span>
                  </div>
                  <div class="h-1.5 bg-[#0B0E13] border border-slate-800 rounded-full overflow-hidden">
                    <div
                      :class="['h-full rounded-full transition-all', cpuColor(tr.cpu_percent)]"
                      :style="{ width: cpuBarWidth(tr.cpu_percent) }"
                    />
                  </div>
                </div>

                <!-- Memory bar -->
                <div class="mb-2">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Memory</span>
                    <span class="text-xs text-slate-400 tabular-nums">
                      {{ formatBytes(tr.mem_used) }} / {{ formatBytes(tr.mem_limit) }}
                      <span v-if="tr.mem_percent !== null" class="text-slate-600 ml-1">({{ tr.mem_percent.toFixed(1) }}%)</span>
                    </span>
                  </div>
                  <div class="h-1.5 bg-[#0B0E13] border border-slate-800 rounded-full overflow-hidden">
                    <div
                      :class="['h-full rounded-full transition-all', memColor(tr.mem_percent)]"
                      :style="{ width: tr.mem_percent !== null ? `${Math.min(tr.mem_percent, 100)}%` : '0%' }"
                    />
                  </div>
                </div>

                <!-- Network -->
                <div v-if="tr.net_rx_bytes !== null && tr.net_rx_bytes >= 0" class="flex items-center gap-4 text-xs text-slate-500">
                  <span>RX {{ formatBytes(tr.net_rx_bytes) }}</span>
                  <span>TX {{ formatBytes(tr.net_tx_bytes) }}</span>
                </div>
              </div>
            </div>
          </FeatureGate>
        </template>
      </div>
    </template>

    <!-- Error / no data -->
    <div v-else class="flex items-center justify-center py-16">
      <span class="text-sm text-slate-500">Service not found.</span>
    </div>
  </div>
</template>
